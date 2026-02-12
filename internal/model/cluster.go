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
	var total ResourceQuantity
	for i := range cs.DaemonSets {
		total.CPUMillis += cs.DaemonSets[i].EffectiveCPUMillis
		total.MemoryBytes += cs.DaemonSets[i].EffectiveMemoryBytes
	}
	return total
}
