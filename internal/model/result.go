package model

import "time"

// NodeAllocation represents one provisioned node and the workloads placed on it.
type NodeAllocation struct {
	Template  NodeTemplate      `json:"template"`
	Workloads []WorkloadProfile `json:"workloads"`
	UsedCPU   int64             `json:"used_cpu_millis"`
	UsedMem   int64             `json:"used_memory_bytes"`
	PodCount  int32             `json:"pod_count"`

	// Derived metrics
	CPUUtilization float64 `json:"cpu_utilization"` // 0.0 - 1.0
	MemUtilization float64 `json:"mem_utilization"` // 0.0 - 1.0
}

// CPUWaste returns unused CPU millicores on this node.
func (na NodeAllocation) CPUWaste() int64 {
	return na.Template.AllocatableCPUMillis - na.UsedCPU
}

// MemWaste returns unused memory bytes on this node.
func (na NodeAllocation) MemWaste() int64 {
	return na.Template.AllocatableMemoryBytes - na.UsedMem
}

// InstanceConfig describes the instance type mix used for a simulation run.
type InstanceConfig struct {
	InstanceTypes []NodeTemplate `json:"instance_types"`
	SpotRatio     float64        `json:"spot_ratio"`
	Strategy      string         `json:"strategy"` // "homogeneous" or "mixed"
}

// Label returns a human-readable label for this configuration.
func (ic InstanceConfig) Label() string {
	if ic.Strategy == "homogeneous" && len(ic.InstanceTypes) == 1 {
		return ic.InstanceTypes[0].InstanceType
	}
	label := ""
	for i, t := range ic.InstanceTypes {
		if i > 0 {
			label += " + "
		}
		label += t.InstanceType
	}
	return label + " (mixed)"
}

// FragmentationReport details resource waste patterns.
type FragmentationReport struct {
	// Stranded: one dimension nearly full, the other underused
	StrandedCPUMillis   int64   `json:"stranded_cpu_millis"`
	StrandedMemoryBytes int64   `json:"stranded_memory_bytes"`

	// Fraction of nodes below 50% utilization on either dimension
	UnderutilizedNodeFraction float64 `json:"underutilized_node_fraction"`

	// 1.0 = perfectly balanced CPU/mem ratio across nodes
	ResourceBalanceScore float64 `json:"resource_balance_score"`
}

// ScalingEfficiency captures how well an instance type handles the observed
// scaling range (min → max nodes over the metrics window).
type ScalingEfficiency struct {
	ScalingRatio     float64 `json:"scaling_ratio"`      // min_nodes/max_nodes observed
	ObservedMinNodes int     `json:"observed_min_nodes"`
	ObservedMaxNodes int     `json:"observed_max_nodes"`
	EstTroughNodes   int     `json:"est_trough_nodes"`   // max(MinNodes, ceil(peakNodes * ratio))
	EstTroughCPUUtil float64 `json:"est_trough_cpu_util"` // 0.0–1.0
}

// SimulationResult captures the outcome of a single bin-packing run.
type SimulationResult struct {
	// The instance configuration used
	InstanceConfig InstanceConfig `json:"instance_config"`

	// Node allocations
	Nodes []NodeAllocation `json:"nodes"`

	// Aggregate metrics
	TotalNodes  int     `json:"total_nodes"`
	TotalCost   float64 `json:"total_monthly_cost"`
	TotalCPU    int64   `json:"total_cpu_millis"`
	TotalMemory int64   `json:"total_memory_bytes"`
	UsedCPU     int64   `json:"used_cpu_millis"`
	UsedMemory  int64   `json:"used_memory_bytes"`

	// Efficiency
	AvgCPUUtilization float64             `json:"avg_cpu_utilization"`
	AvgMemUtilization float64             `json:"avg_mem_utilization"`
	Fragmentation     FragmentationReport `json:"fragmentation"`

	// Scaling efficiency (nil if no aggregate metrics available)
	ScalingEfficiency *ScalingEfficiency `json:"scaling_efficiency,omitempty"`

	// Pods that could not be placed
	UnschedulablePods []WorkloadProfile `json:"unschedulable_pods,omitempty"`

	// Duration of the simulation
	SimulationDuration time.Duration `json:"simulation_duration"`
}

// ScoringWeights configures the relative importance of scoring dimensions.
type ScoringWeights struct {
	Cost          float64 `yaml:"cost" json:"cost"`
	Utilization   float64 `yaml:"utilization" json:"utilization"`
	Fragmentation float64 `yaml:"fragmentation" json:"fragmentation"`
	Resilience    float64 `yaml:"resilience" json:"resilience"`
}

// DefaultScoringWeights returns the default scoring weights.
func DefaultScoringWeights() ScoringWeights {
	return ScoringWeights{
		Cost:          0.40,
		Utilization:   0.30,
		Fragmentation: 0.15,
		Resilience:    0.15,
	}
}

// Recommendation is the final ranked output presented to the user.
type Recommendation struct {
	Rank             int              `json:"rank"`
	SimulationResult SimulationResult `json:"simulation_result"`

	// Cost analysis
	MonthlyCost   float64 `json:"monthly_cost"`
	CostVsBaseline float64 `json:"cost_vs_baseline_pct"` // Negative = savings
	AnnualSavings float64 `json:"annual_savings"`

	// Efficiency scores (0-100)
	OverallScore       float64 `json:"overall_score"`
	CostScore          float64 `json:"cost_score"`
	UtilizationScore   float64 `json:"utilization_score"`
	ResilienceScore    float64 `json:"resilience_score"`
	FragmentationScore float64 `json:"fragmentation_score"`

	// Human-readable rationale
	Rationale string   `json:"rationale"`
	Warnings  []string `json:"warnings,omitempty"`
}

// AlternativeArch summarises the top pick for an alternative CPU architecture.
type AlternativeArch struct {
	Architecture string         `json:"architecture"` // e.g. "arm64 (Graviton)", "amd64 (AMD)"
	TopPick      Recommendation `json:"top_pick"`
	Savings      float64        `json:"savings_pct"` // % cheaper vs primary top pick (positive = cheaper)
}
