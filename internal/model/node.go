package model

// CapacityType represents the EC2 purchasing option.
type CapacityType string

const (
	CapacityOnDemand CapacityType = "on-demand"
	CapacitySpot     CapacityType = "spot"
)

// Architecture represents the CPU architecture.
type Architecture string

const (
	ArchAMD64 Architecture = "amd64"
	ArchARM64 Architecture = "arm64"
)

// NodeTemplate represents a candidate EC2 instance type for bin-packing simulation.
type NodeTemplate struct {
	// EC2 identity
	InstanceType   string       // e.g., "m7g.xlarge"
	InstanceFamily string       // e.g., "m7g"
	Generation     int          // e.g., 7
	Size           string       // e.g., "xlarge"
	Architecture   Architecture // amd64 or arm64

	// Hardware capacity (raw)
	VCPUs     int32
	MemoryMiB int64

	// Kubernetes-adjusted capacity (after system reservation)
	AllocatableCPUMillis   int64
	AllocatableMemoryBytes int64

	// Networking / pod density
	MaxENIs    int32
	IPv4PerENI int32
	MaxPods    int32 // Computed from ENI formula

	// Pricing (hourly)
	OnDemandPricePerHour float64
	SpotPricePerHour     float64
	CapacityType         CapacityType

	// Metadata
	CurrentGeneration bool
	Region            string
}

// EffectivePricePerHour returns the price based on the configured CapacityType.
func (n NodeTemplate) EffectivePricePerHour() float64 {
	if n.CapacityType == CapacitySpot && n.SpotPricePerHour > 0 {
		return n.SpotPricePerHour
	}
	return n.OnDemandPricePerHour
}

// AllocatableResources returns the allocatable capacity as a ResourceQuantity.
func (n NodeTemplate) AllocatableResources() ResourceQuantity {
	return ResourceQuantity{
		CPUMillis:   n.AllocatableCPUMillis,
		MemoryBytes: n.AllocatableMemoryBytes,
	}
}

// MonthlyCost returns the estimated monthly cost (730 hours/month).
func (n NodeTemplate) MonthlyCost() float64 {
	return n.EffectivePricePerHour() * 730.0
}

// HoursPerMonth is the standard number of hours used for monthly cost estimates.
const HoursPerMonth = 730.0
