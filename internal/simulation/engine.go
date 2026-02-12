package simulation

import (
	"context"
	"fmt"
	"math"
	"runtime"
	"sync"
	"time"

	"github.com/guimove/clusterfit/internal/model"
)

// Engine orchestrates bin-packing simulations across multiple instance configurations.
type Engine struct {
	Packer      BinPacker
	Scorer      *Scorer
	Parallelism int
}

// NewEngine creates a simulation engine.
func NewEngine(packer BinPacker, scorer *Scorer) *Engine {
	return &Engine{
		Packer:      packer,
		Scorer:      scorer,
		Parallelism: runtime.NumCPU(),
	}
}

// Scenario defines a single simulation run configuration.
type Scenario struct {
	Name          string
	InstanceTypes []model.NodeTemplate
	Strategy      string  // "homogeneous" or "mixed"
	SpotRatio     float64
	MinNodes      int
}

// RunAll executes all scenarios and returns ranked recommendations.
func (e *Engine) RunAll(
	ctx context.Context,
	scenarios []Scenario,
	state model.ClusterState,
) ([]model.Recommendation, error) {
	if len(scenarios) == 0 {
		return nil, fmt.Errorf("no simulation scenarios provided")
	}

	results := make([]model.SimulationResult, len(scenarios))
	errs := make([]error, len(scenarios))

	// Run simulations in parallel using a worker pool
	sem := make(chan struct{}, e.Parallelism)
	var wg sync.WaitGroup

	for i, sc := range scenarios {
		wg.Add(1)
		go func(idx int, scenario Scenario) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			result, err := e.runOne(ctx, scenario, state)
			results[idx] = result
			errs[idx] = err
		}(i, sc)
	}

	wg.Wait()

	// Collect successful results
	var successful []model.SimulationResult
	for i, err := range errs {
		if err != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			continue // Skip failed scenarios
		}
		successful = append(successful, results[i])
	}

	if len(successful) == 0 {
		return nil, fmt.Errorf("all simulation scenarios failed")
	}

	// Score and rank
	recs := e.Scorer.RankResults(successful, nil)
	return recs, nil
}

// runOne executes a single simulation scenario.
func (e *Engine) runOne(
	ctx context.Context,
	scenario Scenario,
	state model.ClusterState,
) (model.SimulationResult, error) {
	start := time.Now()

	input := PackInput{
		Workloads:      state.Workloads,
		DaemonSets:     state.DaemonSets,
		NodeTemplates:  scenario.InstanceTypes,
		SystemReserved: state.SystemReserved,
		MinNodes:       scenario.MinNodes,
		SpotRatio:      scenario.SpotRatio,
	}

	result, err := e.Packer.Pack(ctx, input)
	if err != nil {
		return model.SimulationResult{}, fmt.Errorf("packing scenario %q: %w", scenario.Name, err)
	}

	duration := time.Since(start)

	// Build simulation result
	simResult := buildSimulationResult(result, scenario, duration, state.AggregateMetrics)
	return simResult, nil
}

// buildSimulationResult computes aggregate metrics from pack results.
func buildSimulationResult(
	pr *PackResult,
	scenario Scenario,
	duration time.Duration,
	aggMetrics *model.ClusterAggregateMetrics,
) model.SimulationResult {
	sr := model.SimulationResult{
		InstanceConfig: model.InstanceConfig{
			InstanceTypes: scenario.InstanceTypes,
			SpotRatio:     scenario.SpotRatio,
			Strategy:      scenario.Strategy,
		},
		Nodes:              pr.Nodes,
		TotalNodes:         len(pr.Nodes),
		UnschedulablePods:  pr.UnschedulablePods,
		SimulationDuration: duration,
	}

	if len(pr.Nodes) == 0 {
		return sr
	}

	var totalCPUUtil, totalMemUtil float64
	for i := range pr.Nodes {
		n := &pr.Nodes[i]
		sr.TotalCPU += n.Template.AllocatableCPUMillis
		sr.TotalMemory += n.Template.AllocatableMemoryBytes
		sr.UsedCPU += n.UsedCPU
		sr.UsedMemory += n.UsedMem
		sr.TotalCost += n.Template.MonthlyCost()
		totalCPUUtil += n.CPUUtilization
		totalMemUtil += n.MemUtilization
	}

	nf := float64(len(pr.Nodes))
	sr.AvgCPUUtilization = totalCPUUtil / nf
	sr.AvgMemUtilization = totalMemUtil / nf

	// Fragmentation analysis
	sr.Fragmentation = AnalyzeFragmentation(pr.Nodes)

	// Scaling efficiency: estimate trough utilization using aggregate metrics
	if aggMetrics != nil && aggMetrics.MaxNodeCount > 0 && len(pr.Nodes) > 0 {
		ratio := aggMetrics.ScalingRatio()
		troughNodes := int(math.Ceil(float64(sr.TotalNodes) * ratio))
		if scenario.MinNodes > 0 && troughNodes < scenario.MinNodes {
			troughNodes = scenario.MinNodes
		}
		troughCPUUtil := 0.0
		if troughNodes > 0 && pr.Nodes[0].Template.AllocatableCPUMillis > 0 {
			allocPerNode := pr.Nodes[0].Template.AllocatableCPUMillis
			troughCPUUtil = (aggMetrics.P95CPUCores * ratio * 1000) / float64(int64(troughNodes)*allocPerNode)
			if troughCPUUtil > 1.0 {
				troughCPUUtil = 1.0
			}
		}
		sr.ScalingEfficiency = &model.ScalingEfficiency{
			ScalingRatio:     ratio,
			ObservedMinNodes: aggMetrics.MinNodeCount,
			ObservedMaxNodes: aggMetrics.MaxNodeCount,
			EstTroughNodes:   troughNodes,
			EstTroughCPUUtil: troughCPUUtil,
		}
	}

	return sr
}

// GenerateScenarios creates simulation scenarios from a list of instance templates.
// For "homogeneous" strategy: one scenario per instance type.
// For "mixed" strategy: one scenario per instance family (all sizes within the family).
// For "both": all of the above.
// minNodes is the HA constraint applied to every scenario.
func GenerateScenarios(templates []model.NodeTemplate, strategy string, spotRatio float64, minNodes int) []Scenario {
	var scenarios []Scenario

	if strategy == "homogeneous" || strategy == "both" {
		for i := range templates {
			t := templates[i]
			scenarios = append(scenarios, Scenario{
				Name:          fmt.Sprintf("homogeneous-%s", t.InstanceType),
				InstanceTypes: []model.NodeTemplate{t},
				Strategy:      "homogeneous",
				SpotRatio:     spotRatio,
				MinNodes:      minNodes,
			})
		}
	}

	if strategy == "mixed" || strategy == "both" {
		// Group by family
		families := make(map[string][]model.NodeTemplate)
		for i := range templates {
			fam := templates[i].InstanceFamily
			families[fam] = append(families[fam], templates[i])
		}

		for fam, types := range families {
			if len(types) < 2 {
				continue // Need at least 2 sizes for a mixed scenario
			}
			scenarios = append(scenarios, Scenario{
				Name:          fmt.Sprintf("mixed-%s", fam),
				InstanceTypes: types,
				Strategy:      "mixed",
				SpotRatio:     spotRatio,
				MinNodes:      minNodes,
			})
		}
	}

	return scenarios
}
