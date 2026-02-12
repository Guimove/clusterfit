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

	// Step 2: Fetch instance types
	_, _ = fmt.Fprintf(o.Writer, "Fetching EC2 instance types for %s...\n", cfg.Cluster.Region)

	archs := make([]model.Architecture, len(cfg.Instances.Architectures))
	for i, a := range cfg.Instances.Architectures {
		archs[i] = model.Architecture(a)
	}

	filter := aws.InstanceFilter{
		Families:              cfg.Instances.Families,
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

	_, _ = fmt.Fprintf(o.Writer, "Evaluating %d instance types across %d scenarios...\n",
		len(templates), 0) // Will be updated below

	// Step 3: Generate and run simulations
	scenarios := simulation.GenerateScenarios(templates, cfg.Simulation.Strategy, cfg.Simulation.SpotRatio)

	_, _ = fmt.Fprintf(o.Writer, "Running %d simulation scenarios...\n", len(scenarios))

	weights := model.ScoringWeights{
		Cost:          cfg.Scoring.Weights.Cost,
		Utilization:   cfg.Scoring.Weights.Utilization,
		Fragmentation: cfg.Scoring.Weights.Fragmentation,
		Resilience:    cfg.Scoring.Weights.Resilience,
	}

	packer := &simulation.BestFitDecreasing{}
	scorer := simulation.NewScorer(weights)
	engine := simulation.NewEngine(packer, scorer)

	recs, err := engine.RunAll(ctx, scenarios, *state)
	if err != nil {
		return nil, fmt.Errorf("running simulations: %w", err)
	}

	// Limit to top N
	if cfg.Output.TopN > 0 && len(recs) > cfg.Output.TopN {
		recs = recs[:cfg.Output.TopN]
	}

	// Step 4: Report
	reporter := report.NewReporter(cfg.Output.Format, o.Writer)
	meta := report.ReportMeta{
		ClusterName:  state.ClusterName,
		Region:       state.Region,
		CollectedAt:  state.CollectedAt,
		WindowStart:  opts.Window.Start,
		WindowEnd:    opts.Window.End,
		Percentile:   cfg.Metrics.Percentile,
		TotalPods:    state.WorkloadCount(),
		TotalDaemons: len(state.DaemonSets),
		Strategy:     cfg.Simulation.Strategy,
	}

	if err := reporter.Report(ctx, recs, meta); err != nil {
		return nil, fmt.Errorf("generating report: %w", err)
	}

	return recs, nil
}

// Simulate runs simulations on a pre-collected cluster state.
func (o *Orchestrator) Simulate(ctx context.Context, state *model.ClusterState, instanceTypes []model.NodeTemplate) ([]model.Recommendation, error) {
	cfg := o.Config

	scenarios := simulation.GenerateScenarios(instanceTypes, cfg.Simulation.Strategy, cfg.Simulation.SpotRatio)

	weights := model.ScoringWeights{
		Cost:          cfg.Scoring.Weights.Cost,
		Utilization:   cfg.Scoring.Weights.Utilization,
		Fragmentation: cfg.Scoring.Weights.Fragmentation,
		Resilience:    cfg.Scoring.Weights.Resilience,
	}

	packer := &simulation.BestFitDecreasing{}
	scorer := simulation.NewScorer(weights)
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
