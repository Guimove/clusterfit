package cmd

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	awspkg "github.com/guimove/clusterfit/internal/aws"
	"github.com/guimove/clusterfit/internal/orchestrator"
)

var recommendCmd = &cobra.Command{
	Use:   "recommend",
	Short: "Analyze cluster metrics and recommend EC2 instance types",
	Long: `Connects to Prometheus, collects pod resource usage metrics, fetches EC2
instance types and pricing, runs bin-packing simulations, and outputs
a ranked list of instance type recommendations.`,
	RunE: runRecommend,
}

func init() {
	f := recommendCmd.Flags()
	f.Duration("window", 7*24*time.Hour, "metrics lookback window")
	f.Float64("percentile", 0.95, "percentile for sizing (0.0-1.0)")
	f.StringSlice("families", nil, "EC2 instance families to consider")
	f.StringSlice("architectures", nil, "CPU architectures (amd64, arm64)")
	f.Float64("spot-ratio", 0, "fraction of nodes to run as spot (0.0-1.0)")
	f.StringSlice("exclude-namespaces", nil, "namespaces to exclude")
	f.Int("top", 5, "number of recommendations to show")
	f.String("output", "table", "output format: table, json, markdown")
	f.String("output-file", "", "write output to file")
	f.Bool("no-cache", false, "disable caching")

	rootCmd.AddCommand(recommendCmd)
}

func runRecommend(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	// Apply flag overrides
	if w, _ := cmd.Flags().GetDuration("window"); w > 0 {
		cfg.Metrics.Window = w
	}
	if p, _ := cmd.Flags().GetFloat64("percentile"); cmd.Flags().Changed("percentile") {
		cfg.Metrics.Percentile = p
	}
	if fam, _ := cmd.Flags().GetStringSlice("families"); len(fam) > 0 {
		cfg.Instances.Families = fam
	}
	if archs, _ := cmd.Flags().GetStringSlice("architectures"); len(archs) > 0 {
		cfg.Instances.Architectures = archs
	}
	if sr, _ := cmd.Flags().GetFloat64("spot-ratio"); cmd.Flags().Changed("spot-ratio") {
		cfg.Simulation.SpotRatio = sr
	}
	if ns, _ := cmd.Flags().GetStringSlice("exclude-namespaces"); len(ns) > 0 {
		cfg.Metrics.ExcludeNamespaces = ns
	}
	if n, _ := cmd.Flags().GetInt("top"); cmd.Flags().Changed("top") {
		cfg.Output.TopN = n
	}
	if f, _ := cmd.Flags().GetString("output"); cmd.Flags().Changed("output") {
		cfg.Output.Format = f
	}

	if err := cfg.Validate(); err != nil {
		return err
	}

	// Create metrics collector
	collector, err := resolveCollector(ctx)
	if err != nil {
		return fmt.Errorf("creating metrics collector: %w", err)
	}

	// Verify connectivity
	if err := collector.Ping(ctx); err != nil {
		return fmt.Errorf("connecting to Prometheus: %w", err)
	}

	// Create AWS provider
	cacheDir := ""
	if noCache, _ := cmd.Flags().GetBool("no-cache"); !noCache {
		home, _ := os.UserHomeDir()
		cacheDir = filepath.Join(home, ".cache", "clusterfit")
	}

	provider, err := awspkg.NewAWSProvider(ctx, cfg.Cluster.Region, cacheDir)
	if err != nil {
		return fmt.Errorf("creating AWS provider: %w", err)
	}

	// Handle output file
	w := os.Stdout
	if outFile, _ := cmd.Flags().GetString("output-file"); outFile != "" {
		f, err := os.Create(outFile)
		if err != nil {
			return fmt.Errorf("creating output file: %w", err)
		}
		defer f.Close()
		w = f
	}

	// Run orchestrator
	orch := orchestrator.New(collector, provider, cfg)
	orch.Writer = w

	_, err = orch.Recommend(ctx)
	return err
}
