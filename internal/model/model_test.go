package model

import (
	"testing"
)

func TestResourceQuantity_Add(t *testing.T) {
	a := ResourceQuantity{CPUMillis: 500, MemoryBytes: 1024}
	b := ResourceQuantity{CPUMillis: 300, MemoryBytes: 2048}
	result := a.Add(b)

	if result.CPUMillis != 800 {
		t.Errorf("CPUMillis: got %d, want 800", result.CPUMillis)
	}
	if result.MemoryBytes != 3072 {
		t.Errorf("MemoryBytes: got %d, want 3072", result.MemoryBytes)
	}
}

func TestResourceQuantity_Sub(t *testing.T) {
	a := ResourceQuantity{CPUMillis: 500, MemoryBytes: 2048}
	b := ResourceQuantity{CPUMillis: 300, MemoryBytes: 1024}
	result := a.Sub(b)

	if result.CPUMillis != 200 {
		t.Errorf("CPUMillis: got %d, want 200", result.CPUMillis)
	}
	if result.MemoryBytes != 1024 {
		t.Errorf("MemoryBytes: got %d, want 1024", result.MemoryBytes)
	}
}

func TestResourceQuantity_FitsIn(t *testing.T) {
	tests := []struct {
		name     string
		r        ResourceQuantity
		capacity ResourceQuantity
		want     bool
	}{
		{"exact fit", ResourceQuantity{1000, 1024}, ResourceQuantity{1000, 1024}, true},
		{"smaller", ResourceQuantity{500, 512}, ResourceQuantity{1000, 1024}, true},
		{"cpu exceeds", ResourceQuantity{1500, 512}, ResourceQuantity{1000, 1024}, false},
		{"mem exceeds", ResourceQuantity{500, 2048}, ResourceQuantity{1000, 1024}, false},
		{"both exceed", ResourceQuantity{1500, 2048}, ResourceQuantity{1000, 1024}, false},
		{"zero fits anything", ResourceQuantity{0, 0}, ResourceQuantity{1000, 1024}, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.r.FitsIn(tt.capacity); got != tt.want {
				t.Errorf("FitsIn() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestResourceQuantity_IsZero(t *testing.T) {
	if !(ResourceQuantity{0, 0}).IsZero() {
		t.Error("expected zero to be zero")
	}
	if (ResourceQuantity{1, 0}).IsZero() {
		t.Error("expected non-zero CPU to not be zero")
	}
	if (ResourceQuantity{0, 1}).IsZero() {
		t.Error("expected non-zero memory to not be zero")
	}
}

func TestPercentileValues_AtPercentile(t *testing.T) {
	pv := PercentileValues{P50: 10, P95: 20, P99: 30, Max: 50}

	tests := []struct {
		pct  float64
		want float64
	}{
		{0.50, 10},
		{0.75, 20},
		{0.95, 20},
		{0.97, 30},
		{0.99, 30},
		{1.0, 50},
	}

	for _, tt := range tests {
		got := pv.AtPercentile(tt.pct)
		if got != tt.want {
			t.Errorf("AtPercentile(%v) = %v, want %v", tt.pct, got, tt.want)
		}
	}
}

func TestNodeTemplate_EffectivePricePerHour(t *testing.T) {
	n := NodeTemplate{
		OnDemandPricePerHour: 0.10,
		SpotPricePerHour:     0.03,
	}

	n.CapacityType = CapacityOnDemand
	if got := n.EffectivePricePerHour(); got != 0.10 {
		t.Errorf("on-demand: got %v, want 0.10", got)
	}

	n.CapacityType = CapacitySpot
	if got := n.EffectivePricePerHour(); got != 0.03 {
		t.Errorf("spot: got %v, want 0.03", got)
	}

	// Spot with zero price falls back to on-demand
	n.SpotPricePerHour = 0
	if got := n.EffectivePricePerHour(); got != 0.10 {
		t.Errorf("spot zero fallback: got %v, want 0.10", got)
	}
}

func TestNodeTemplate_MonthlyCost(t *testing.T) {
	n := NodeTemplate{
		OnDemandPricePerHour: 0.10,
		CapacityType:         CapacityOnDemand,
	}
	expected := 0.10 * 730.0
	if got := n.MonthlyCost(); got != expected {
		t.Errorf("MonthlyCost() = %v, want %v", got, expected)
	}
}

func TestClusterState_Totals(t *testing.T) {
	cs := ClusterState{
		Workloads: []WorkloadProfile{
			{EffectiveCPUMillis: 500, EffectiveMemoryBytes: 1024},
			{EffectiveCPUMillis: 300, EffectiveMemoryBytes: 2048},
		},
		DaemonSets: []WorkloadProfile{
			{EffectiveCPUMillis: 100, EffectiveMemoryBytes: 512},
		},
	}

	if got := cs.TotalEffectiveCPU(); got != 800 {
		t.Errorf("TotalEffectiveCPU() = %d, want 800", got)
	}
	if got := cs.TotalEffectiveMemory(); got != 3072 {
		t.Errorf("TotalEffectiveMemory() = %d, want 3072", got)
	}
	if got := cs.WorkloadCount(); got != 2 {
		t.Errorf("WorkloadCount() = %d, want 2", got)
	}

	overhead := cs.DaemonSetOverhead()
	if overhead.CPUMillis != 100 || overhead.MemoryBytes != 512 {
		t.Errorf("DaemonSetOverhead() = %+v, want {100, 512}", overhead)
	}
}

func TestInstanceConfig_Label(t *testing.T) {
	ic := InstanceConfig{
		InstanceTypes: []NodeTemplate{{InstanceType: "m5.xlarge"}},
		Strategy:      "homogeneous",
	}
	if got := ic.Label(); got != "m5.xlarge" {
		t.Errorf("Label() = %q, want %q", got, "m5.xlarge")
	}

	ic2 := InstanceConfig{
		InstanceTypes: []NodeTemplate{
			{InstanceType: "m5.xlarge"},
			{InstanceType: "r5.xlarge"},
		},
		Strategy: "mixed",
	}
	want := "m5.xlarge + r5.xlarge (mixed)"
	if got := ic2.Label(); got != want {
		t.Errorf("Label() = %q, want %q", got, want)
	}
}

func TestNodeAllocation_Waste(t *testing.T) {
	na := NodeAllocation{
		Template: NodeTemplate{
			AllocatableCPUMillis:   4000,
			AllocatableMemoryBytes: 8 * 1024 * 1024 * 1024,
		},
		UsedCPU: 3000,
		UsedMem: 6 * 1024 * 1024 * 1024,
	}

	if got := na.CPUWaste(); got != 1000 {
		t.Errorf("CPUWaste() = %d, want 1000", got)
	}
	if got := na.MemWaste(); got != 2*1024*1024*1024 {
		t.Errorf("MemWaste() = %d, want %d", got, 2*1024*1024*1024)
	}
}

func TestDefaultScoringWeights(t *testing.T) {
	w := DefaultScoringWeights()
	sum := w.Cost + w.Utilization + w.Fragmentation + w.Resilience
	if sum < 0.99 || sum > 1.01 {
		t.Errorf("weights sum to %v, want ~1.0", sum)
	}
}
