package orchestrator

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/guimove/clusterfit/internal/aws"
	"github.com/guimove/clusterfit/internal/config"
	"github.com/guimove/clusterfit/internal/metrics"
	"github.com/guimove/clusterfit/internal/model"
	"github.com/guimove/clusterfit/internal/report"
	"github.com/guimove/clusterfit/internal/simulation"
)

// Orchestrator coordinates the end-to-end recommendation pipeline.
type Orchestrator struct {
	Collector metrics.MetricsCollector
	Provider  aws.PricingProvider
	Config    config.Config
	Writer    io.Writer
}

// New creates an orchestrator with the given dependencies.
func New(collector metrics.MetricsCollector, provider aws.PricingProvider, cfg config.Config) *Orchestrator {
	return &Orchestrator{
		Collector: collector,
		Provider:  provider,
		Config:    cfg,
		Writer:    os.Stdout,
	}
}

// Recommend runs the full pipeline: collect → fetch instances → simulate → rank → report.
func (o *Orchestrator) Recommend(ctx context.Context) ([]model.Recommendation, error) {
	cfg := o.Config

	// Step 1: Collect metrics
	_, _ = fmt.Fprintf(o.Writer, "Collecting metrics from %s backend...\n", o.Collector.BackendType())

	now := time.Now()
	opts := metrics.CollectOptions{
		Window: model.TimeWindow{
			Start: now.Add(-cfg.Metrics.Window),
			End:   now,
			Step:  cfg.Metrics.Step,
		},
		ExcludeNamespaces: cfg.Metrics.ExcludeNamespaces,
		Percentile:        cfg.Metrics.Percentile,
		StepInterval:      cfg.Metrics.Step,
	}

	state, err := o.Collector.Collect(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("collecting metrics: %w", err)
	}

	state.ClusterName = cfg.Cluster.Name
	state.Region = cfg.Cluster.Region
	state.SystemReserved = model.ResourceQuantity{
		CPUMillis:   cfg.Simulation.SystemReserved.CPUMillis,
		MemoryBytes: cfg.Simulation.SystemReserved.MemoryMiB * 1024 * 1024,
	}

	_, _ = fmt.Fprintf(o.Writer, "Found %d workloads and %d DaemonSets\n",
		state.WorkloadCount(), len(state.DaemonSets))

	// Step 2: Auto-classify workloads if families are not explicitly set
	autoClassified := len(cfg.Instances.Families) == 0
	var workloadClass model.WorkloadClass
	var gibPerVCPU float64

	if autoClassified {
		workloadClass, gibPerVCPU = state.ClassifyWorkloads()
		cfg.Instances.Families = model.FamiliesForClass(workloadClass, "intel")
		// Always include general-purpose as a fallback when auto-classifying
		// to an extreme (C or R). Per-pod requests may skew the bin-packing
		// ratio away from the aggregate classification, and M-series provides
		// a safe middle ground that the scorer can evaluate.
		if workloadClass != model.WorkloadClassGeneral {
			cfg.Instances.Families = append(cfg.Instances.Families, model.FamiliesForClass(model.WorkloadClassGeneral, "intel")...)
		}
		_, _ = fmt.Fprintf(o.Writer, "Workload profile: %s (%.1f GiB/vCPU) → families %v\n",
			workloadClass, gibPerVCPU, cfg.Instances.Families)
	}

	// Step 3: Fetch instance types and run primary simulation
	_, _ = fmt.Fprintf(o.Writer, "Fetching EC2 instance types for %s...\n", cfg.Cluster.Region)

	weights := model.ScoringWeights{
		Cost:          cfg.Scoring.Weights.Cost,
		Utilization:   cfg.Scoring.Weights.Utilization,
		Fragmentation: cfg.Scoring.Weights.Fragmentation,
		Resilience:    cfg.Scoring.Weights.Resilience,
	}

	recs, err := o.runSimulation(ctx, cfg, state, weights, cfg.Instances.Families, []model.Architecture{model.ArchAMD64})
	if err != nil {
		return nil, err
	}

	// Step 4: Run architecture alternatives (only when auto-classified)
	var alternatives []model.AlternativeArch
	if autoClassified && len(recs) > 0 {
		primaryCost := recs[0].MonthlyCost

		type altDef struct {
			label    string
			families []string
			arch     model.Architecture
		}
		gravitonFamilies := model.FamiliesForClass(workloadClass, "graviton")
		amdFamilies := model.FamiliesForClass(workloadClass, "amd")
		if workloadClass != model.WorkloadClassGeneral {
			gravitonFamilies = append(gravitonFamilies, model.FamiliesForClass(model.WorkloadClassGeneral, "graviton")...)
			amdFamilies = append(amdFamilies, model.FamiliesForClass(model.WorkloadClassGeneral, "amd")...)
		}
		altDefs := []altDef{
			{"arm64 (Graviton)", gravitonFamilies, model.ArchARM64},
			{"amd64 (AMD)", amdFamilies, model.ArchAMD64},
		}

		for _, ad := range altDefs {
			altRecs, altErr := o.runSimulation(ctx, cfg, state, weights, ad.families, []model.Architecture{ad.arch})
			if altErr != nil || len(altRecs) == 0 {
				continue
			}
			savings := 0.0
			if primaryCost > 0 {
				savings = (primaryCost - altRecs[0].MonthlyCost) / primaryCost * 100
			}
			alternatives = append(alternatives, model.AlternativeArch{
				Architecture: ad.label,
				TopPick:      altRecs[0],
				Savings:      savings,
			})
		}
	}

	// Step 5: Report
	reporter := report.NewReporter(cfg.Output.Format, o.Writer)
	meta := report.ReportMeta{
		ClusterName:      state.ClusterName,
		Region:           state.Region,
		CollectedAt:      state.CollectedAt,
		WindowStart:      opts.Window.Start,
		WindowEnd:        opts.Window.End,
		Percentile:       cfg.Metrics.Percentile,
		TotalPods:        state.WorkloadCount(),
		TotalDaemons:     len(state.DaemonSets),
		Strategy:         cfg.Simulation.Strategy,
		MinNodes:         cfg.Simulation.MinNodes,
		AggregateMetrics: state.AggregateMetrics,
	}
	if autoClassified {
		meta.WorkloadClass = string(workloadClass)
		meta.GiBPerVCPU = gibPerVCPU
		meta.Alternatives = alternatives
	}

	if err := reporter.Report(ctx, recs, meta); err != nil {
		return nil, fmt.Errorf("generating report: %w", err)
	}

	return recs, nil
}

// runSimulation fetches instance types for the given families/architectures and runs the simulation pipeline.
func (o *Orchestrator) runSimulation(ctx context.Context, cfg config.Config, state *model.ClusterState, weights model.ScoringWeights, families []string, archs []model.Architecture) ([]model.Recommendation, error) {
	filter := aws.InstanceFilter{
		Families:              families,
		MinVCPUs:              cfg.Instances.MinVCPUs,
		MaxVCPUs:              cfg.Instances.MaxVCPUs,
		Architectures:         archs,
		CurrentGenerationOnly: cfg.Instances.CurrentGenerationOnly,
		ExcludeBareMetal:      cfg.Instances.ExcludeBareMetal,
		ExcludeBurstable:      cfg.Instances.ExcludeBurstable,
	}

	templates, err := o.Provider.GetInstanceTypes(ctx, filter)
	if err != nil {
		return nil, fmt.Errorf("fetching instance types: %w", err)
	}

	scenarios := simulation.GenerateScenarios(templates, cfg.Simulation.Strategy, cfg.Simulation.SpotRatio, cfg.Simulation.MinNodes)

	_, _ = fmt.Fprintf(o.Writer, "Simulating %d scenarios across %d instance types...\n",
		len(scenarios), len(templates))

	packer := &simulation.BestFitDecreasing{}
	scorer := simulation.NewScorer(weights)
	scorer.DaemonSetCount = len(state.DaemonSets)
	scorer.AggregateMetrics = state.AggregateMetrics
	engine := simulation.NewEngine(packer, scorer)

	recs, err := engine.RunAll(ctx, scenarios, *state)
	if err != nil {
		return nil, fmt.Errorf("running simulations: %w", err)
	}

	if cfg.Output.TopN > 0 && len(recs) > cfg.Output.TopN {
		recs = recs[:cfg.Output.TopN]
	}

	return recs, nil
}

// Simulate runs simulations on a pre-collected cluster state.
func (o *Orchestrator) Simulate(ctx context.Context, state *model.ClusterState, instanceTypes []model.NodeTemplate) ([]model.Recommendation, error) {
	cfg := o.Config

	scenarios := simulation.GenerateScenarios(instanceTypes, cfg.Simulation.Strategy, cfg.Simulation.SpotRatio, cfg.Simulation.MinNodes)

	weights := model.ScoringWeights{
		Cost:          cfg.Scoring.Weights.Cost,
		Utilization:   cfg.Scoring.Weights.Utilization,
		Fragmentation: cfg.Scoring.Weights.Fragmentation,
		Resilience:    cfg.Scoring.Weights.Resilience,
	}

	packer := &simulation.BestFitDecreasing{}
	scorer := simulation.NewScorer(weights)
	scorer.AggregateMetrics = state.AggregateMetrics
	engine := simulation.NewEngine(packer, scorer)

	recs, err := engine.RunAll(ctx, scenarios, *state)
	if err != nil {
		return nil, fmt.Errorf("running simulations: %w", err)
	}

	if cfg.Output.TopN > 0 && len(recs) > cfg.Output.TopN {
		recs = recs[:cfg.Output.TopN]
	}

	return recs, nil
}
