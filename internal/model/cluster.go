package model

import "time"

// TimeWindow represents a time range for metrics collection.
type TimeWindow struct {
	Start time.Time
	End   time.Time
	Step  time.Duration
}

// Duration returns the length of the time window.
func (tw TimeWindow) Duration() time.Duration {
	return tw.End.Sub(tw.Start)
}

// ClusterAggregateMetrics holds cluster-wide aggregate metrics over the full
// metrics window, capturing scaling peaks that per-pod snapshots miss.
type ClusterAggregateMetrics struct {
	P95CPUCores    float64 `json:"p95_cpu_cores"`
	P95MemoryBytes float64 `json:"p95_memory_bytes"`
	MinNodeCount   int     `json:"min_node_count"`
	MaxNodeCount   int     `json:"max_node_count"`
}

// ScalingRatio returns the ratio of min to max node count (0.0–1.0).
// Returns 1.0 if metrics are unavailable or max is zero.
func (cam *ClusterAggregateMetrics) ScalingRatio() float64 {
	if cam == nil || cam.MaxNodeCount == 0 {
		return 1.0
	}
	return float64(cam.MinNodeCount) / float64(cam.MaxNodeCount)
}

// ClusterState is a point-in-time snapshot of cluster workloads and configuration,
// serving as input to the simulation engine.
type ClusterState struct {
	// When the snapshot was taken
	CollectedAt time.Time `json:"collected_at"`

	// The metrics time window that was analyzed
	MetricsWindow TimeWindow `json:"metrics_window"`

	// All workload profiles discovered (excluding DaemonSets)
	Workloads []WorkloadProfile `json:"workloads"`

	// DaemonSet workloads (must run on every node)
	DaemonSets []WorkloadProfile `json:"daemon_sets"`

	// System overhead per node (kubelet, kube-proxy, etc.)
	SystemReserved ResourceQuantity `json:"system_reserved"`

	// Cluster-wide aggregate metrics (P95 CPU/mem, node count range)
	AggregateMetrics *ClusterAggregateMetrics `json:"aggregate_metrics,omitempty"`

	// Cluster metadata
	ClusterName string `json:"cluster_name"`
	Region      string `json:"region"`
	KubeVersion string `json:"kube_version,omitempty"`
}

// TotalEffectiveCPU returns the sum of all workload effective CPU demand in millicores.
func (cs ClusterState) TotalEffectiveCPU() int64 {
	var total int64
	for i := range cs.Workloads {
		total += cs.Workloads[i].EffectiveCPUMillis
	}
	return total
}

// TotalEffectiveMemory returns the sum of all workload effective memory demand in bytes.
func (cs ClusterState) TotalEffectiveMemory() int64 {
	var total int64
	for i := range cs.Workloads {
		total += cs.Workloads[i].EffectiveMemoryBytes
	}
	return total
}

// WorkloadCount returns the number of non-DaemonSet workloads.
func (cs ClusterState) WorkloadCount() int {
	return len(cs.Workloads)
}

// DaemonSetOverhead returns the total resources consumed by DaemonSets per node.
func (cs ClusterState) DaemonSetOverhead() ResourceQuantity {
	return SumEffectiveResources(cs.DaemonSets)
}

// SumEffectiveResources returns the total effective CPU and memory across a slice of workloads.
func SumEffectiveResources(wps []WorkloadProfile) ResourceQuantity {
	var total ResourceQuantity
	for i := range wps {
		total.CPUMillis += wps[i].EffectiveCPUMillis
		total.MemoryBytes += wps[i].EffectiveMemoryBytes
	}
	return total
}

// WorkloadClass describes the dominant resource profile of a cluster's workloads.
type WorkloadClass string

const (
	WorkloadClassCompute WorkloadClass = "compute-optimized"
	WorkloadClassGeneral WorkloadClass = "general-purpose"
	WorkloadClassMemory  WorkloadClass = "memory-optimized"
)

// ClassifyWorkloads computes the aggregate GiB-per-vCPU ratio and returns
// the workload class plus the ratio value.
//   - ratio < 3.0  → compute-optimized (C-series)
//   - 3.0 ≤ ratio ≤ 6.0 → general-purpose (M-series)
//   - ratio > 6.0  → memory-optimized (R-series)
//
// When cluster-level aggregate metrics are available (P95 CPU and memory over
// the full window), they are preferred over per-pod effective totals because
// per-pod sizing uses max(request, usage) which inflates CPU when pods
// over-request relative to actual usage.
func (cs ClusterState) ClassifyWorkloads() (WorkloadClass, float64) {
	var vcpus, gib float64

	// Prefer cluster-level aggregate P95 when available — it reflects actual
	// usage patterns without request inflation.
	if cs.AggregateMetrics != nil && cs.AggregateMetrics.P95CPUCores > 0 && cs.AggregateMetrics.P95MemoryBytes > 0 {
		vcpus = cs.AggregateMetrics.P95CPUCores
		gib = cs.AggregateMetrics.P95MemoryBytes / (1024 * 1024 * 1024)
	} else {
		cpuMillis := cs.TotalEffectiveCPU()
		memBytes := cs.TotalEffectiveMemory()
		if cpuMillis == 0 {
			return WorkloadClassGeneral, 4.0
		}
		vcpus = float64(cpuMillis) / 1000.0
		gib = float64(memBytes) / (1024 * 1024 * 1024)
	}

	if vcpus == 0 {
		return WorkloadClassGeneral, 4.0
	}

	ratio := gib / vcpus

	switch {
	case ratio < 3.0:
		return WorkloadClassCompute, ratio
	case ratio > 6.0:
		return WorkloadClassMemory, ratio
	default:
		return WorkloadClassGeneral, ratio
	}
}

// FamiliesForClass returns the EC2 instance families matching a workload class
// and architecture vendor. Valid arch values: "intel", "amd", "graviton".
func FamiliesForClass(class WorkloadClass, arch string) []string {
	type key struct {
		class WorkloadClass
		arch  string
	}
	mapping := map[key][]string{
		{WorkloadClassCompute, "intel"}:   {"c7i", "c6i"},
		{WorkloadClassCompute, "amd"}:     {"c7a", "c6a"},
		{WorkloadClassCompute, "graviton"}: {"c7g", "c6g"},
		{WorkloadClassGeneral, "intel"}:   {"m7i", "m6i"},
		{WorkloadClassGeneral, "amd"}:     {"m7a", "m6a"},
		{WorkloadClassGeneral, "graviton"}: {"m7g", "m6g"},
		{WorkloadClassMemory, "intel"}:    {"r7i", "r6i"},
		{WorkloadClassMemory, "amd"}:      {"r7a", "r6a"},
		{WorkloadClassMemory, "graviton"}: {"r7g", "r6g"},
	}
	return mapping[key{class, arch}]
}
