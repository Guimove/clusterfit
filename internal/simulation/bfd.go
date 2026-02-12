package simulation

import (
	"context"
	"math"
	"sort"

	"github.com/guimove/clusterfit/internal/model"
)

// BestFitDecreasing implements a multi-dimensional best-fit-decreasing bin-packing algorithm.
type BestFitDecreasing struct{}

// Name returns the strategy name.
func (b *BestFitDecreasing) Name() string { return "best-fit-decreasing" }

// nodeState tracks the current allocation state of a node during packing.
type nodeState struct {
	template     model.NodeTemplate
	workloads    []model.WorkloadProfile
	remainingCPU int64
	remainingMem int64
	podCount     int32
}

// Pack places workloads onto nodes using the BFD algorithm.
func (b *BestFitDecreasing) Pack(ctx context.Context, input PackInput) (*PackResult, error) {
	if len(input.NodeTemplates) == 0 {
		return &PackResult{UnschedulablePods: input.Workloads}, nil
	}

	// Pre-compute DaemonSet overhead (applied to every node)
	dsOverhead := daemonSetOverhead(input.DaemonSets)

	// Sort workloads by dominance score (largest first)
	workloads := make([]model.WorkloadProfile, len(input.Workloads))
	copy(workloads, input.Workloads)
	sortByDominance(workloads, input.NodeTemplates)

	var nodes []nodeState
	var unschedulable []model.WorkloadProfile

	for i := range workloads {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}

		w := &workloads[i]

		// Find the best-fitting existing node
		bestIdx := -1
		bestScore := math.MaxFloat64

		for j := range nodes {
			if !canFit(&nodes[j], w) {
				continue
			}
			score := compositeRemaining(&nodes[j], w)
			if score < bestScore {
				bestScore = score
				bestIdx = j
			}
		}

		if bestIdx >= 0 {
			place(&nodes[bestIdx], w)
			continue
		}

		// No existing node fits — open a new one
		if input.MaxNodes > 0 && len(nodes) >= input.MaxNodes {
			unschedulable = append(unschedulable, *w)
			continue
		}

		tmpl := selectBestTemplate(input.NodeTemplates, w, dsOverhead, input.SystemReserved)
		if tmpl == nil {
			unschedulable = append(unschedulable, *w)
			continue
		}

		n := openNode(*tmpl, dsOverhead, input.SystemReserved)
		place(&n, w)
		nodes = append(nodes, n)
	}

	// Apply spot/on-demand ratio
	if input.SpotRatio > 0 {
		applySpotRatio(nodes, input.SpotRatio)
	}

	// Build result
	allocations := make([]model.NodeAllocation, len(nodes))
	for i, n := range nodes {
		alloc := n.template.AllocatableResources()
		usedCPU := alloc.CPUMillis - n.remainingCPU
		usedMem := alloc.MemoryBytes - n.remainingMem

		allocations[i] = model.NodeAllocation{
			Template:  n.template,
			Workloads: n.workloads,
			UsedCPU:   usedCPU,
			UsedMem:   usedMem,
			PodCount:  n.podCount,
		}
		if alloc.CPUMillis > 0 {
			allocations[i].CPUUtilization = float64(usedCPU) / float64(alloc.CPUMillis)
		}
		if alloc.MemoryBytes > 0 {
			allocations[i].MemUtilization = float64(usedMem) / float64(alloc.MemoryBytes)
		}
	}

	return &PackResult{
		Nodes:             allocations,
		UnschedulablePods: unschedulable,
	}, nil
}

// daemonSetOverhead computes the total resources consumed by DaemonSets per node.
func daemonSetOverhead(daemons []model.WorkloadProfile) model.ResourceQuantity {
	var total model.ResourceQuantity
	for i := range daemons {
		total.CPUMillis += daemons[i].EffectiveCPUMillis
		total.MemoryBytes += daemons[i].EffectiveMemoryBytes
	}
	return total
}

// sortByDominance sorts workloads so the most demanding pods come first.
// Dominance = max(cpuFraction, memFraction) relative to the largest node.
func sortByDominance(workloads []model.WorkloadProfile, templates []model.NodeTemplate) {
	maxCPU, maxMem := largestNodeCapacity(templates)
	if maxCPU == 0 || maxMem == 0 {
		return
	}

	sort.SliceStable(workloads, func(i, j int) bool {
		di := dominance(&workloads[i], maxCPU, maxMem)
		dj := dominance(&workloads[j], maxCPU, maxMem)
		return di > dj
	})
}

func dominance(w *model.WorkloadProfile, maxCPU, maxMem int64) float64 {
	cpuFrac := float64(w.EffectiveCPUMillis) / float64(maxCPU)
	memFrac := float64(w.EffectiveMemoryBytes) / float64(maxMem)
	return math.Max(cpuFrac, memFrac)
}

func largestNodeCapacity(templates []model.NodeTemplate) (int64, int64) {
	var maxCPU, maxMem int64
	for i := range templates {
		if templates[i].AllocatableCPUMillis > maxCPU {
			maxCPU = templates[i].AllocatableCPUMillis
		}
		if templates[i].AllocatableMemoryBytes > maxMem {
			maxMem = templates[i].AllocatableMemoryBytes
		}
	}
	return maxCPU, maxMem
}

// canFit checks whether workload w fits in node n (CPU, memory, and pod count).
func canFit(n *nodeState, w *model.WorkloadProfile) bool {
	return w.EffectiveCPUMillis <= n.remainingCPU &&
		w.EffectiveMemoryBytes <= n.remainingMem &&
		n.podCount < n.template.MaxPods
}

// compositeRemaining returns a scalar measuring how tightly packed a node would be
// after placing workload w. Lower = tighter fit = preferred.
func compositeRemaining(n *nodeState, w *model.WorkloadProfile) float64 {
	alloc := n.template.AllocatableResources()
	if alloc.CPUMillis == 0 || alloc.MemoryBytes == 0 {
		return math.MaxFloat64
	}
	cpuAfter := float64(n.remainingCPU-w.EffectiveCPUMillis) / float64(alloc.CPUMillis)
	memAfter := float64(n.remainingMem-w.EffectiveMemoryBytes) / float64(alloc.MemoryBytes)
	// Euclidean distance from origin — penalizes imbalance
	return math.Sqrt(cpuAfter*cpuAfter + memAfter*memAfter)
}

// selectBestTemplate picks the smallest instance type that fits the workload
// after accounting for DaemonSet overhead and system reserved.
func selectBestTemplate(
	templates []model.NodeTemplate,
	w *model.WorkloadProfile,
	dsOverhead, sysReserved model.ResourceQuantity,
) *model.NodeTemplate {
	var best *model.NodeTemplate
	bestCost := math.MaxFloat64

	for i := range templates {
		t := &templates[i]
		availCPU := t.AllocatableCPUMillis - dsOverhead.CPUMillis - sysReserved.CPUMillis
		availMem := t.AllocatableMemoryBytes - dsOverhead.MemoryBytes - sysReserved.MemoryBytes

		if w.EffectiveCPUMillis > availCPU || w.EffectiveMemoryBytes > availMem {
			continue
		}

		cost := t.OnDemandPricePerHour
		if cost < bestCost {
			bestCost = cost
			best = t
		}
	}
	return best
}

// openNode creates a new nodeState with DaemonSet overhead and system reserved subtracted.
func openNode(tmpl model.NodeTemplate, dsOverhead, sysReserved model.ResourceQuantity) nodeState {
	return nodeState{
		template:     tmpl,
		remainingCPU: tmpl.AllocatableCPUMillis - dsOverhead.CPUMillis - sysReserved.CPUMillis,
		remainingMem: tmpl.AllocatableMemoryBytes - dsOverhead.MemoryBytes - sysReserved.MemoryBytes,
		podCount:     0,
	}
}

// place puts a workload onto a node, updating remaining resources.
func place(n *nodeState, w *model.WorkloadProfile) {
	n.workloads = append(n.workloads, *w)
	n.remainingCPU -= w.EffectiveCPUMillis
	n.remainingMem -= w.EffectiveMemoryBytes
	n.podCount++
}

// applySpotRatio assigns CapacitySpot to the appropriate fraction of nodes.
// Nodes with fewer, smaller workloads are preferred for spot.
func applySpotRatio(nodes []nodeState, spotRatio float64) {
	spotCount := int(math.Round(float64(len(nodes)) * spotRatio))
	if spotCount <= 0 {
		return
	}
	if spotCount > len(nodes) {
		spotCount = len(nodes)
	}

	// Sort by suitability for spot (lower total resource usage = more suitable)
	indices := make([]int, len(nodes))
	for i := range indices {
		indices[i] = i
	}
	sort.SliceStable(indices, func(i, j int) bool {
		ai := nodes[indices[i]]
		aj := nodes[indices[j]]
		return (ai.template.AllocatableCPUMillis - ai.remainingCPU) <
			(aj.template.AllocatableCPUMillis - aj.remainingCPU)
	})

	for i, idx := range indices {
		if i < spotCount {
			nodes[idx].template.CapacityType = model.CapacitySpot
		} else {
			nodes[idx].template.CapacityType = model.CapacityOnDemand
		}
	}
}
