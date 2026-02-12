package simulation

import (
	"math"

	"github.com/guimove/clusterfit/internal/model"
)

const (
	// HighUtilThreshold is the utilization level above which a resource dimension
	// is considered nearly full (used for stranded-resource detection and warnings).
	HighUtilThreshold = 0.85

	// LowUtilThreshold is the utilization level below which a resource dimension
	// is considered underutilized.
	LowUtilThreshold = 0.50

	// CriticalMemUtilThreshold triggers a memory OOM-risk warning.
	CriticalMemUtilThreshold = 0.90

	// HighSpotRatio is the spot fraction above which an interruption-risk warning fires.
	HighSpotRatio = 0.50
)

// AnalyzeFragmentation computes fragmentation metrics for a set of node allocations.
func AnalyzeFragmentation(nodes []model.NodeAllocation) model.FragmentationReport {
	if len(nodes) == 0 {
		return model.FragmentationReport{ResourceBalanceScore: 1.0}
	}

	var report model.FragmentationReport
	var underutilized int

	for i := range nodes {
		n := &nodes[i]
		alloc := n.Template.AllocatableResources()
		if alloc.CPUMillis == 0 || alloc.MemoryBytes == 0 {
			continue
		}

		cpuUtil := float64(n.UsedCPU) / float64(alloc.CPUMillis)
		memUtil := float64(n.UsedMem) / float64(alloc.MemoryBytes)

		// Stranded resources: one dimension nearly full, other underused
		if cpuUtil > HighUtilThreshold && memUtil < LowUtilThreshold {
			report.StrandedMemoryBytes += alloc.MemoryBytes - n.UsedMem
		}
		if memUtil > HighUtilThreshold && cpuUtil < LowUtilThreshold {
			report.StrandedCPUMillis += alloc.CPUMillis - n.UsedCPU
		}

		// Under-utilized: either dimension below threshold
		if cpuUtil < LowUtilThreshold || memUtil < LowUtilThreshold {
			underutilized++
		}

		// Resource balance: how close CPU% and memory% are
		report.ResourceBalanceScore += 1.0 - math.Abs(cpuUtil-memUtil)
	}

	n := float64(len(nodes))
	report.UnderutilizedNodeFraction = float64(underutilized) / n
	report.ResourceBalanceScore /= n

	return report
}
