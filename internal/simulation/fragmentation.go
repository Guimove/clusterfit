package simulation

import (
	"math"

	"github.com/guimove/clusterfit/internal/model"
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

		// Stranded resources: one dimension nearly full (>85%), other underused (<50%)
		if cpuUtil > 0.85 && memUtil < 0.50 {
			report.StrandedMemoryBytes += alloc.MemoryBytes - n.UsedMem
		}
		if memUtil > 0.85 && cpuUtil < 0.50 {
			report.StrandedCPUMillis += alloc.CPUMillis - n.UsedCPU
		}

		// Under-utilized: either dimension below 50%
		if cpuUtil < 0.50 || memUtil < 0.50 {
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
