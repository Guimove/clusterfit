package metrics

import (
	"context"
	"errors"
	"time"

	"github.com/guimove/clusterfit/internal/model"
)

var (
	ErrPrometheusUnreachable = errors.New("prometheus endpoint unreachable")
	ErrNoMetricsFound        = errors.New("no pod metrics found for the specified criteria")
)

// MetricsCollector abstracts the collection of pod-level resource usage metrics.
type MetricsCollector interface {
	// Collect gathers workload profiles for all pods in the cluster.
	Collect(ctx context.Context, opts CollectOptions) (*model.ClusterState, error)

	// Ping validates connectivity to the metrics backend.
	Ping(ctx context.Context) error

	// BackendType returns the detected backend type.
	BackendType() string
}

// CollectOptions configures metrics collection.
type CollectOptions struct {
	Window            model.TimeWindow
	Namespaces        []string      // Empty = all namespaces
	ExcludeNamespaces []string      // Namespaces to exclude
	LabelSelector     string        // Optional label filter
	Percentile        float64       // Which percentile for effective sizing (default 0.95)
	StepInterval      time.Duration // PromQL step interval
}
