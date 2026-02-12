package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/guimove/clusterfit/internal/model"
	"github.com/guimove/clusterfit/internal/simulation"
)

var whatifCmd = &cobra.Command{
	Use:   "what-if",
	Short: "Compare instance type scenarios side by side",
	Long: `Runs the same workload through multiple instance configurations
and compares them directly. Useful for evaluating migration to
a different instance family or Graviton.`,
	RunE: runWhatIf,
}

func init() {
	f := whatifCmd.Flags()
	f.String("input", "", "path to cluster state JSON file (required)")
	f.String("baseline", "", "baseline instance type (e.g., m5.xlarge)")
	f.StringSlice("candidates", nil, "candidate instance types to compare")
	f.Float64("scale-factor", 1.0, "multiply workload count by this factor")
	f.String("output", "table", "output format: table, json")

	_ = whatifCmd.MarkFlagRequired("input")
	rootCmd.AddCommand(whatifCmd)
}

func runWhatIf(cmd *cobra.Command, args []string) error {
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

	// Scale workloads if requested
	scaleFactor, _ := cmd.Flags().GetFloat64("scale-factor")
	if scaleFactor > 1.0 {
		state.Workloads = scaleWorkloads(state.Workloads, scaleFactor)
	}

	// Build scenarios from baseline and candidates
	allTemplates := defaultSimulationTemplates()
	templateMap := make(map[string]model.NodeTemplate)
	for _, t := range allTemplates {
		templateMap[t.InstanceType] = t
	}

	var scenarios []simulation.Scenario

	baseline, _ := cmd.Flags().GetString("baseline")
	if baseline != "" {
		if t, ok := templateMap[baseline]; ok {
			scenarios = append(scenarios, simulation.Scenario{
				Name:          baseline + " (baseline)",
				InstanceTypes: []model.NodeTemplate{t},
				Strategy:      "homogeneous",
			})
		} else {
			return fmt.Errorf("unknown baseline instance type: %s", baseline)
		}
	}

	candidates, _ := cmd.Flags().GetStringSlice("candidates")
	for _, c := range candidates {
		if t, ok := templateMap[c]; ok {
			scenarios = append(scenarios, simulation.Scenario{
				Name:          c,
				InstanceTypes: []model.NodeTemplate{t},
				Strategy:      "homogeneous",
			})
		} else {
			fmt.Fprintf(os.Stderr, "Warning: unknown instance type %q, skipping\n", c)
		}
	}

	if len(scenarios) == 0 {
		return fmt.Errorf("no valid scenarios to compare")
	}

	weights := model.DefaultScoringWeights()
	packer := &simulation.BestFitDecreasing{}
	scorer := simulation.NewScorer(weights)
	engine := simulation.NewEngine(packer, scorer)

	recs, err := engine.RunAll(ctx, scenarios, state)
	if err != nil {
		return err
	}

	// Output comparison
	outputFmt, _ := cmd.Flags().GetString("output")
	if outputFmt == "json" {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		return enc.Encode(recs)
	}

	fmt.Printf("\nWhat-If Comparison (%d workloads", len(state.Workloads))
	if scaleFactor > 1.0 {
		fmt.Printf(", scaled %.1fx", scaleFactor)
	}
	fmt.Printf(")\n")
	fmt.Printf("%s\n\n", strings.Repeat("=", 80))

	fmt.Printf("%-25s %6s %7s %7s %8s %6s\n",
		"Configuration", "Nodes", "CPU%", "Mem%", "$/month", "Score")
	fmt.Printf("%s\n", strings.Repeat("-", 80))

	for _, rec := range recs {
		sr := rec.SimulationResult
		tag := ""
		if strings.Contains(sr.InstanceConfig.Label(), "baseline") {
			tag = " <--"
		}
		fmt.Printf("%-25s %6d %6.1f%% %6.1f%% %8.0f %6.1f%s\n",
			sr.InstanceConfig.Label(),
			sr.TotalNodes,
			sr.AvgCPUUtilization*100,
			sr.AvgMemUtilization*100,
			rec.MonthlyCost,
			rec.OverallScore,
			tag,
		)
	}

	fmt.Printf("%s\n", strings.Repeat("-", 80))

	if len(recs) >= 2 && baseline != "" {
		best := recs[0]
		if best.MonthlyCost < recs[len(recs)-1].MonthlyCost {
			savings := recs[len(recs)-1].MonthlyCost - best.MonthlyCost
			fmt.Printf("\nBest option saves $%.0f/month ($%.0f/year) vs worst\n",
				savings, savings*12)
		}
	}

	return nil
}

func scaleWorkloads(workloads []model.WorkloadProfile, factor float64) []model.WorkloadProfile {
	target := int(float64(len(workloads)) * factor)
	scaled := make([]model.WorkloadProfile, target)
	for i := 0; i < target; i++ {
		scaled[i] = workloads[i%len(workloads)]
	}
	return scaled
}
