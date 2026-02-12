package model

// ResourceQuantity represents a CPU/memory quantity with millicpu and bytes precision.
type ResourceQuantity struct {
	CPUMillis   int64 // CPU in millicores (1000 = 1 vCPU)
	MemoryBytes int64 // Memory in bytes
}

// Add returns the sum of two ResourceQuantity values.
func (r ResourceQuantity) Add(other ResourceQuantity) ResourceQuantity {
	return ResourceQuantity{
		CPUMillis:   r.CPUMillis + other.CPUMillis,
		MemoryBytes: r.MemoryBytes + other.MemoryBytes,
	}
}

// Sub returns the difference of two ResourceQuantity values.
func (r ResourceQuantity) Sub(other ResourceQuantity) ResourceQuantity {
	return ResourceQuantity{
		CPUMillis:   r.CPUMillis - other.CPUMillis,
		MemoryBytes: r.MemoryBytes - other.MemoryBytes,
	}
}

// FitsIn returns true if this quantity fits within the given capacity.
func (r ResourceQuantity) FitsIn(capacity ResourceQuantity) bool {
	return r.CPUMillis <= capacity.CPUMillis && r.MemoryBytes <= capacity.MemoryBytes
}

// IsZero returns true if both dimensions are zero.
func (r ResourceQuantity) IsZero() bool {
	return r.CPUMillis == 0 && r.MemoryBytes == 0
}

// PercentileValues holds observed resource usage at multiple percentiles.
type PercentileValues struct {
	P50 float64
	P95 float64
	P99 float64
	Max float64
}

// AtPercentile returns the value at the given percentile (0.0 to 1.0).
// Falls back to the nearest available percentile.
func (p PercentileValues) AtPercentile(pct float64) float64 {
	switch {
	case pct <= 0.50:
		return p.P50
	case pct <= 0.95:
		return p.P95
	case pct <= 0.99:
		return p.P99
	default:
		return p.Max
	}
}

// WorkloadProfile represents the resource footprint of a single pod or replica group,
// derived from historical metrics.
type WorkloadProfile struct {
	// Identity
	Namespace string
	Name      string // Pod name or controller name
	OwnerKind string // Deployment, StatefulSet, DaemonSet, Job, etc.
	OwnerName string

	// Requests and limits as declared in the pod spec
	Requested ResourceQuantity
	Limits    ResourceQuantity

	// Observed usage from Prometheus (percentile-based)
	CPUUsage    PercentileValues // In cores (float64)
	MemoryUsage PercentileValues // In bytes (float64)

	// Derived sizing at the chosen percentile — used for bin-packing
	EffectiveCPUMillis   int64
	EffectiveMemoryBytes int64

	// Replica count (for controller-managed workloads)
	Replicas int32

	// Scheduling constraints
	NodeSelector map[string]string
	Tolerations  []string
	Architecture Architecture // Required architecture (empty = any)

	// Whether this is a DaemonSet pod (runs on every node)
	IsDaemonSet bool

	// Whether this pod had no observed metrics (used request values)
	NoMetrics bool
}
