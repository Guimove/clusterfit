package metrics

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	promapi "github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
	prommodel "github.com/prometheus/common/model"

	"github.com/guimove/clusterfit/internal/model"
)

// PrometheusCollector collects metrics from Prometheus, Thanos, or Cortex.
type PrometheusCollector struct {
	api      promv1.API
	endpoint string
	backend  string
	timeout  time.Duration
}

// PrometheusOption configures the Prometheus collector.
type PrometheusOption func(*PrometheusCollector)

// WithTimeout sets the query timeout.
func WithTimeout(d time.Duration) PrometheusOption {
	return func(c *PrometheusCollector) { c.timeout = d }
}

// NewPrometheusCollector creates a collector connected to the given endpoint.
func NewPrometheusCollector(endpoint string, opts ...PrometheusOption) (*PrometheusCollector, error) {
	client, err := promapi.NewClient(promapi.Config{
		Address: endpoint,
	})
	if err != nil {
		return nil, fmt.Errorf("creating prometheus client: %w", err)
	}

	c := &PrometheusCollector{
		api:      promv1.NewAPI(client),
		endpoint: endpoint,
		backend:  "prometheus",
		timeout:  60 * time.Second,
	}

	for _, opt := range opts {
		opt(c)
	}

	return c, nil
}

// Ping checks connectivity and detects the backend type.
func (c *PrometheusCollector) Ping(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	// Try a simple query to check connectivity
	_, _, err := c.api.Query(ctx, "up", time.Now())
	if err != nil {
		return fmt.Errorf("%w: %v", ErrPrometheusUnreachable, err)
	}

	// Detect backend
	c.detectBackend(ctx)
	return nil
}

// BackendType returns the detected backend type.
func (c *PrometheusCollector) BackendType() string {
	return c.backend
}

// detectBackend tries to identify Thanos or Cortex.
func (c *PrometheusCollector) detectBackend(ctx context.Context) {
	// Check buildinfo for Thanos/Cortex
	resp, err := http.Get(c.endpoint + "/api/v1/status/buildinfo")
	if err == nil {
		resp.Body.Close()
	}

	// Try Thanos-specific metric
	result, _, err := c.api.Query(ctx, "thanos_store_nodes_total", time.Now())
	if err == nil && result != nil && result.String() != "" {
		c.backend = "thanos"
		return
	}

	// Try Cortex-specific metric
	result, _, err = c.api.Query(ctx, "cortex_ingester_active_series", time.Now())
	if err == nil && result != nil && result.String() != "" {
		c.backend = "cortex"
	}
}

// Collect gathers workload profiles from Prometheus.
func (c *PrometheusCollector) Collect(ctx context.Context, opts CollectOptions) (*model.ClusterState, error) {
	windowStr := formatDuration(opts.Window.Duration())
	stepStr := formatDuration(opts.StepInterval)
	if stepStr == "" {
		stepStr = "5m"
	}

	pct := opts.Percentile
	if pct == 0 {
		pct = 0.95
	}

	// Collect all metrics in parallel
	type queryResult struct {
		name string
		data prommodel.Value
		err  error
	}

	queries := map[string]string{
		"cpu_p50":        queryCPUPercentile(0.50, windowStr, stepStr),
		"cpu_p95":        queryCPUPercentile(0.95, windowStr, stepStr),
		"cpu_p99":        queryCPUPercentile(0.99, windowStr, stepStr),
		"mem_p50":        queryMemoryPercentile(0.50, windowStr, stepStr),
		"mem_p95":        queryMemoryPercentile(0.95, windowStr, stepStr),
		"mem_p99":        queryMemoryPercentile(0.99, windowStr, stepStr),
		"cpu_requests":   queryPodResourceRequests("cpu"),
		"mem_requests":   queryPodResourceRequests("memory"),
		"cpu_limits":     queryPodResourceLimits("cpu"),
		"mem_limits":     queryPodResourceLimits("memory"),
		"pod_owner":      queryPodOwner(),
	}

	results := make(chan queryResult, len(queries))
	queryCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()

	for name, q := range queries {
		go func(n, query string) {
			data, _, err := c.api.Query(queryCtx, query, opts.Window.End)
			results <- queryResult{name: n, data: data, err: err}
		}(name, q)
	}

	collected := make(map[string]prommodel.Value)
	var errs []string
	for i := 0; i < len(queries); i++ {
		r := <-results
		if r.err != nil {
			errs = append(errs, fmt.Sprintf("%s: %v", r.name, r.err))
			continue
		}
		collected[r.name] = r.data
	}

	// Build workload profiles from collected data
	return c.buildClusterState(collected, opts, errs)
}

// podKey creates a unique key for a pod.
type podKey struct {
	Namespace string
	Pod       string
}

// buildClusterState assembles the ClusterState from query results.
func (c *PrometheusCollector) buildClusterState(
	data map[string]prommodel.Value,
	opts CollectOptions,
	queryErrors []string,
) (*model.ClusterState, error) {
	// Index all metrics by (namespace, pod)
	cpuP50 := extractVector(data["cpu_p50"])
	cpuP95 := extractVector(data["cpu_p95"])
	cpuP99 := extractVector(data["cpu_p99"])
	memP50 := extractVector(data["mem_p50"])
	memP95 := extractVector(data["mem_p95"])
	memP99 := extractVector(data["mem_p99"])
	cpuReq := extractVector(data["cpu_requests"])
	memReq := extractVector(data["mem_requests"])
	cpuLim := extractVector(data["cpu_limits"])
	memLim := extractVector(data["mem_limits"])
	owners := extractOwnerInfo(data["pod_owner"])

	// Collect all known pods
	allPods := make(map[podKey]bool)
	for k := range cpuP95 {
		allPods[k] = true
	}
	for k := range memP95 {
		allPods[k] = true
	}
	for k := range cpuReq {
		allPods[k] = true
	}

	if len(allPods) == 0 {
		errDetail := ""
		if len(queryErrors) > 0 {
			errDetail = "; query errors: " + strings.Join(queryErrors, ", ")
		}
		return nil, fmt.Errorf("%w%s", ErrNoMetricsFound, errDetail)
	}

	excludeNS := make(map[string]bool)
	for _, ns := range opts.ExcludeNamespaces {
		excludeNS[ns] = true
	}

	var workloads, daemonSets []model.WorkloadProfile

	for pk := range allPods {
		if excludeNS[pk.Namespace] {
			continue
		}

		wp := model.WorkloadProfile{
			Namespace: pk.Namespace,
			Name:      pk.Pod,
			CPUUsage: model.PercentileValues{
				P50: cpuP50[pk],
				P95: cpuP95[pk],
				P99: cpuP99[pk],
				Max: cpuP99[pk], // Use P99 as max approximation
			},
			MemoryUsage: model.PercentileValues{
				P50: memP50[pk],
				P95: memP95[pk],
				P99: memP99[pk],
				Max: memP99[pk],
			},
			Requested: model.ResourceQuantity{
				CPUMillis:   int64(cpuReq[pk] * 1000),
				MemoryBytes: int64(memReq[pk]),
			},
			Limits: model.ResourceQuantity{
				CPUMillis:   int64(cpuLim[pk] * 1000),
				MemoryBytes: int64(memLim[pk]),
			},
		}

		// Determine effective sizing based on configured percentile
		cpuAtPct := wp.CPUUsage.AtPercentile(opts.Percentile)
		memAtPct := wp.MemoryUsage.AtPercentile(opts.Percentile)

		// Use the max of (request, observed usage at percentile) for bin-packing
		wp.EffectiveCPUMillis = int64(math.Max(float64(wp.Requested.CPUMillis), cpuAtPct*1000))
		wp.EffectiveMemoryBytes = int64(math.Max(float64(wp.Requested.MemoryBytes), memAtPct))

		// If no metrics at all, mark and use requests
		if cpuP95[pk] == 0 && memP95[pk] == 0 {
			wp.NoMetrics = true
			wp.EffectiveCPUMillis = wp.Requested.CPUMillis
			wp.EffectiveMemoryBytes = wp.Requested.MemoryBytes
		}

		// Minimum effective values (10m CPU, 64MiB memory)
		if wp.EffectiveCPUMillis < 10 {
			wp.EffectiveCPUMillis = 10
		}
		if wp.EffectiveMemoryBytes < 64*1024*1024 {
			wp.EffectiveMemoryBytes = 64 * 1024 * 1024
		}

		// Check owner info for DaemonSet
		if owner, ok := owners[pk]; ok {
			wp.OwnerKind = owner.Kind
			wp.OwnerName = owner.Name
			if owner.Kind == "DaemonSet" {
				wp.IsDaemonSet = true
			}
		}

		if wp.IsDaemonSet {
			daemonSets = append(daemonSets, wp)
		} else {
			workloads = append(workloads, wp)
		}
	}

	return &model.ClusterState{
		CollectedAt:   time.Now(),
		MetricsWindow: opts.Window,
		Workloads:     workloads,
		DaemonSets:    daemonSets,
	}, nil
}

// ownerInfo holds parsed pod owner reference data.
type ownerInfo struct {
	Kind string
	Name string
}

// extractVector converts a Prometheus Value to a map of (namespace, pod) → float64.
func extractVector(v prommodel.Value) map[podKey]float64 {
	result := make(map[podKey]float64)
	if v == nil {
		return result
	}

	vec, ok := v.(prommodel.Vector)
	if !ok {
		return result
	}

	for _, sample := range vec {
		ns := string(sample.Metric["namespace"])
		pod := string(sample.Metric["pod"])
		if ns == "" || pod == "" {
			continue
		}
		result[podKey{ns, pod}] = float64(sample.Value)
	}
	return result
}

// extractOwnerInfo parses pod owner references from kube_pod_owner metric.
func extractOwnerInfo(v prommodel.Value) map[podKey]ownerInfo {
	result := make(map[podKey]ownerInfo)
	if v == nil {
		return result
	}

	vec, ok := v.(prommodel.Vector)
	if !ok {
		return result
	}

	for _, sample := range vec {
		ns := string(sample.Metric["namespace"])
		pod := string(sample.Metric["pod"])
		kind := string(sample.Metric["owner_kind"])
		name := string(sample.Metric["owner_name"])
		if ns == "" || pod == "" {
			continue
		}
		result[podKey{ns, pod}] = ownerInfo{Kind: kind, Name: name}
	}
	return result
}

// formatDuration formats a time.Duration to a Prometheus-compatible duration string.
func formatDuration(d time.Duration) string {
	if d == 0 {
		return ""
	}
	hours := int(d.Hours())
	if hours >= 24 && hours%24 == 0 {
		return fmt.Sprintf("%dd", hours/24)
	}
	if hours > 0 {
		return fmt.Sprintf("%dh", hours)
	}
	minutes := int(d.Minutes())
	if minutes > 0 {
		return fmt.Sprintf("%dm", minutes)
	}
	return fmt.Sprintf("%ds", int(d.Seconds()))
}
