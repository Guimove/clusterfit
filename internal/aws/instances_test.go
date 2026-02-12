package aws

import (
	"fmt"
	"testing"
)

func TestComputeMaxPods(t *testing.T) {
	tests := []struct {
		name       string
		maxENIs    int32
		ipv4PerENI int32
		want       int32
	}{
		{"m5.large", 3, 10, 29},
		{"m5.xlarge", 4, 15, 59},
		{"t3.micro", 2, 2, 3},
		{"zero ENIs", 0, 0, 110},
		{"huge instance capped", 15, 50, 250},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ComputeMaxPods(tt.maxENIs, tt.ipv4PerENI)
			if got != tt.want {
				t.Errorf("ComputeMaxPods(%d, %d) = %d, want %d",
					tt.maxENIs, tt.ipv4PerENI, got, tt.want)
			}
		})
	}
}

func TestComputeAllocatableCPU(t *testing.T) {
	tests := []struct {
		vcpus int32
		want  int64
	}{
		{1, 940},            // 1000 - 60
		{2, 1930},           // 2000 - 60 - 10
		{4, 3920},           // 4000 - 60 - 10 - 5 - 5
		{8, 7912},           // 8000 - 60 - 10 - 5 - 5 - 2*4
		{16, 15896},         // 16000 - 60 - 10 - 10 - 2*12
		{96, 95736},         // 96000 - 60 - 10 - 10 - 2*92
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_vcpus", tt.vcpus), func(t *testing.T) {
			got := computeAllocatableCPU(tt.vcpus)
			if got != tt.want {
				t.Errorf("computeAllocatableCPU(%d) = %d, want %d", tt.vcpus, got, tt.want)
			}
		})
	}
}

func TestComputeAllocatableMemory(t *testing.T) {
	tests := []struct {
		memMiB int64
		want   int64 // Approximate, check within 1%
	}{
		{8192, 0},  // Just verify non-negative and reasonable
		{16384, 0},
		{32768, 0},
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d_MiB", tt.memMiB), func(t *testing.T) {
			got := computeAllocatableMemory(tt.memMiB)
			total := tt.memMiB * 1024 * 1024
			if got <= 0 || got >= total {
				t.Errorf("computeAllocatableMemory(%d) = %d, expected between 0 and %d",
					tt.memMiB, got, total)
			}
			// Should be at least 60% of total (system reserved shouldn't be more than 40%)
			if float64(got) < float64(total)*0.60 {
				t.Errorf("computeAllocatableMemory(%d) = %d, less than 60%% of total %d",
					tt.memMiB, got, total)
			}
		})
	}
}

func TestParseInstanceType(t *testing.T) {
	tests := []struct {
		input  string
		family string
		gen    int
		size   string
	}{
		{"m5.xlarge", "m5", 5, "xlarge"},
		{"m7g.large", "m7g", 7, "large"},
		{"c6i.2xlarge", "c6i", 6, "2xlarge"},
		{"r5.metal", "r5", 5, "metal"},
		{"t3.micro", "t3", 3, "micro"},
		{"p4d.24xlarge", "p4d", 4, "24xlarge"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			family, gen, size := parseInstanceType(tt.input)
			if family != tt.family {
				t.Errorf("family: got %q, want %q", family, tt.family)
			}
			if gen != tt.gen {
				t.Errorf("generation: got %d, want %d", gen, tt.gen)
			}
			if size != tt.size {
				t.Errorf("size: got %q, want %q", size, tt.size)
			}
		})
	}
}
