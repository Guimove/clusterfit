package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/guimove/clusterfit/internal/metrics"
	"github.com/guimove/clusterfit/internal/model"
)

var inspectCmd = &cobra.Command{
	Use:   "inspect",
	Short: "Collect and display current cluster workload state",
	Long: `Connects to Prometheus, collects pod metrics, and displays the cluster
workload profile. Useful for debugging metrics collection and understanding
resource usage patterns. The JSON output can be fed to 'clusterfit simulate'.`,
	RunE: runInspect,
}

func init() {
	f := inspectCmd.Flags()
	f.Duration("window", 7*24*time.Hour, "metrics lookback window")
	f.Float64("percentile", 0.95, "percentile for sizing")
	f.String("output", "table", "output format: table, json")
	f.String("sort-by", "cpu", "sort workloads by: cpu, memory, name")
	f.String("output-file", "", "write output to file")

	rootCmd.AddCommand(inspectCmd)
}

func runInspect(cmd *cobra.Command, args []string) error {
	ctx := context.Background()

	if w, _ := cmd.Flags().GetDuration("window"); w > 0 {
		cfg.Metrics.Window = w
	}
	if p, _ := cmd.Flags().GetFloat64("percentile"); cmd.Flags().Changed("percentile") {
		cfg.Metrics.Percentile = p
	}

	collector, cleanup, err := resolveCollector(ctx)
	if err != nil {
		return err
	}
	if cleanup != nil {
		defer cleanup()
	}

	if err := collector.Ping(ctx); err != nil {
		return err
	}

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

	state, err := collector.Collect(ctx, opts)
	if err != nil {
		return err
	}

	state.ClusterName = cfg.Cluster.Name
	state.Region = cfg.Cluster.Region

	// Sort workloads
	sortBy, _ := cmd.Flags().GetString("sort-by")
	sortWorkloads(state.Workloads, sortBy)

	outputFmt, _ := cmd.Flags().GetString("output")
	w := os.Stdout
	if outFile, _ := cmd.Flags().GetString("output-file"); outFile != "" {
		f, err := os.Create(outFile)
		if err != nil {
			return err
		}
		defer f.Close()
		w = f
	}

	if outputFmt == "json" {
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		return enc.Encode(state)
	}

	// Table output
	fmt.Fprintf(w, "Cluster: %s (%s)\n", state.ClusterName, state.Region)
	fmt.Fprintf(w, "Backend: %s\n", collector.BackendType())
	fmt.Fprintf(w, "Workloads: %d | DaemonSets: %d\n\n", len(state.Workloads), len(state.DaemonSets))

	fmt.Fprintf(w, "%-30s %-15s %8s %10s %8s %10s %s\n",
		"POD", "NAMESPACE", "CPU(m)", "MEM(MiB)", "REQ_CPU", "REQ_MEM", "FLAGS")
	fmt.Fprintf(w, "%s\n", strings.Repeat("-", 100))

	for _, wp := range state.Workloads {
		flags := ""
		if wp.NoMetrics {
			flags += "[no-metrics]"
		}
		if wp.IsDaemonSet {
			flags += "[ds]"
		}

		fmt.Fprintf(w, "%-30s %-15s %8d %10d %8d %10d %s\n",
			truncate(wp.Name, 30),
			truncate(wp.Namespace, 15),
			wp.EffectiveCPUMillis,
			wp.EffectiveMemoryBytes/(1024*1024),
			wp.Requested.CPUMillis,
			wp.Requested.MemoryBytes/(1024*1024),
			flags,
		)
	}

	fmt.Fprintf(w, "\nTotal effective: CPU=%dm MEM=%dMiB\n",
		state.TotalEffectiveCPU(), state.TotalEffectiveMemory()/(1024*1024))

	return nil
}

func sortWorkloads(wl []model.WorkloadProfile, by string) {
	switch by {
	case "memory":
		sort.Slice(wl, func(i, j int) bool {
			return wl[i].EffectiveMemoryBytes > wl[j].EffectiveMemoryBytes
		})
	case "name":
		sort.Slice(wl, func(i, j int) bool {
			return wl[i].Namespace+"/"+wl[i].Name < wl[j].Namespace+"/"+wl[j].Name
		})
	default: // cpu
		sort.Slice(wl, func(i, j int) bool {
			return wl[i].EffectiveCPUMillis > wl[j].EffectiveCPUMillis
		})
	}
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
