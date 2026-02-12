package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/guimove/clusterfit/internal/model"
	"github.com/guimove/clusterfit/internal/orchestrator"
	"github.com/guimove/clusterfit/internal/report"
)

var simulateCmd = &cobra.Command{
	Use:   "simulate",
	Short: "Run bin-packing simulation on a pre-collected cluster snapshot",
	Long: `Accepts a cluster state JSON file (from 'clusterfit inspect --output json')
and runs simulation without needing live Prometheus access.`,
	RunE: runSimulate,
}

func init() {
	f := simulateCmd.Flags()
	f.String("input", "", "path to cluster state JSON file (required)")
	f.StringSlice("instance-types", nil, "specific instance types to simulate")
	f.String("strategy", "both", "simulation strategy: homogeneous, mixed, or both")
	f.Float64("spot-ratio", 0, "fraction of nodes to run as spot")
	f.String("output", "table", "output format: table, json, markdown")
	f.Int("top", 5, "number of recommendations")

	_ = simulateCmd.MarkFlagRequired("input")
	rootCmd.AddCommand(simulateCmd)
}

func runSimulate(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	inputPath, _ := cmd.Flags().GetString("input")
	data, err := os.ReadFile(inputPath)
	if err != nil {
		return fmt.Errorf("reading input file: %w", err)
	}

	var state model.ClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return fmt.Errorf("parsing cluster state: %w", err)
	}

	if strategy, _ := cmd.Flags().GetString("strategy"); strategy != "" {
		cfg.Simulation.Strategy = strategy
	}
	if sr, _ := cmd.Flags().GetFloat64("spot-ratio"); cmd.Flags().Changed("spot-ratio") {
		cfg.Simulation.SpotRatio = sr
	}
	if f, _ := cmd.Flags().GetString("output"); cmd.Flags().Changed("output") {
		cfg.Output.Format = f
	}
	if n, _ := cmd.Flags().GetInt("top"); cmd.Flags().Changed("top") {
		cfg.Output.TopN = n
	}

	// Build instance templates — in simulate mode, use simple predefined types
	// or load from a separate file
	templates := defaultSimulationTemplates()

	orch := &orchestrator.Orchestrator{Config: cfg, Writer: os.Stdout}
	recs, err := orch.Simulate(ctx, &state, templates)
	if err != nil {
		return err
	}

	reporter := report.NewReporter(cfg.Output.Format, os.Stdout)
	meta := report.ReportMeta{
		ClusterName:  state.ClusterName,
		Region:       state.Region,
		TotalPods:    state.WorkloadCount(),
		TotalDaemons: len(state.DaemonSets),
		Percentile:   cfg.Metrics.Percentile,
		WindowStart:  state.MetricsWindow.Start,
		WindowEnd:    state.MetricsWindow.End,
	}

	return reporter.Report(ctx, recs, meta)
}

// defaultSimulationTemplates returns a set of common instance types for offline simulation.
func defaultSimulationTemplates() []model.NodeTemplate {
	types := []struct {
		name   string
		family string
		gen    int
		size   string
		vcpus  int32
		memMiB int64
		enis   int32
		ipv4   int32
		price  float64
		arch   model.Architecture
	}{
		{"m5.large", "m5", 5, "large", 2, 8192, 3, 10, 0.096, model.ArchAMD64},
		{"m5.xlarge", "m5", 5, "xlarge", 4, 16384, 4, 15, 0.192, model.ArchAMD64},
		{"m5.2xlarge", "m5", 5, "2xlarge", 8, 32768, 4, 15, 0.384, model.ArchAMD64},
		{"m5.4xlarge", "m5", 5, "4xlarge", 16, 65536, 8, 30, 0.768, model.ArchAMD64},
		{"c5.large", "c5", 5, "large", 2, 4096, 3, 10, 0.085, model.ArchAMD64},
		{"c5.xlarge", "c5", 5, "xlarge", 4, 8192, 4, 15, 0.170, model.ArchAMD64},
		{"c5.2xlarge", "c5", 5, "2xlarge", 8, 16384, 4, 15, 0.340, model.ArchAMD64},
		{"r5.large", "r5", 5, "large", 2, 16384, 3, 10, 0.126, model.ArchAMD64},
		{"r5.xlarge", "r5", 5, "xlarge", 4, 32768, 4, 15, 0.252, model.ArchAMD64},
		{"r5.2xlarge", "r5", 5, "2xlarge", 8, 65536, 4, 15, 0.504, model.ArchAMD64},
		{"m6i.large", "m6i", 6, "large", 2, 8192, 3, 10, 0.096, model.ArchAMD64},
		{"m6i.xlarge", "m6i", 6, "xlarge", 4, 16384, 4, 15, 0.192, model.ArchAMD64},
		{"m6i.2xlarge", "m6i", 6, "2xlarge", 8, 32768, 4, 15, 0.384, model.ArchAMD64},
		{"m7g.large", "m7g", 7, "large", 2, 8192, 3, 10, 0.0816, model.ArchARM64},
		{"m7g.xlarge", "m7g", 7, "xlarge", 4, 16384, 4, 15, 0.1632, model.ArchARM64},
		{"m7g.2xlarge", "m7g", 7, "2xlarge", 8, 32768, 4, 15, 0.3264, model.ArchARM64},
	}

	templates := make([]model.NodeTemplate, len(types))
	for i, t := range types {
		templates[i] = model.NodeTemplate{
			InstanceType:           t.name,
			InstanceFamily:         t.family,
			Generation:             t.gen,
			Size:                   t.size,
			VCPUs:                  t.vcpus,
			MemoryMiB:              t.memMiB,
			MaxENIs:                t.enis,
			IPv4PerENI:             t.ipv4,
			MaxPods:                computeMaxPods(t.enis, t.ipv4),
			OnDemandPricePerHour:   t.price,
			Architecture:           t.arch,
			CapacityType:           model.CapacityOnDemand,
			CurrentGeneration:      true,
			Region:                 "us-east-1",
			AllocatableCPUMillis:   int64(t.vcpus)*1000 - 80,  // Simplified reservation
			AllocatableMemoryBytes: t.memMiB*1024*1024 - 512*1024*1024, // Simplified
		}
	}

	return templates
}

func computeMaxPods(enis, ipv4 int32) int32 {
	pods := enis*ipv4 - 1
	if pods > 250 {
		pods = 250
	}
	if pods < 1 {
		pods = 1
	}
	return pods
}
