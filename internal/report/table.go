package report

import (
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/guimove/clusterfit/internal/model"
)

// TableReporter outputs recommendations as a formatted terminal table.
type TableReporter struct {
	w io.Writer
}

func (r *TableReporter) Report(ctx context.Context, recs []model.Recommendation, meta ReportMeta) error {
	// Header
	fmt.Fprintf(r.w, "\n")
	fmt.Fprintf(r.w, "ClusterFit Recommendations\n")
	fmt.Fprintf(r.w, "%s\n", strings.Repeat("=", 60))
	fmt.Fprintf(r.w, "Cluster:     %s\n", meta.ClusterName)
	fmt.Fprintf(r.w, "Region:      %s\n", meta.Region)
	fmt.Fprintf(r.w, "Pods:        %d (+ %d DaemonSets)\n", meta.TotalPods, meta.TotalDaemons)
	fmt.Fprintf(r.w, "Percentile:  p%g\n", meta.Percentile*100)
	fmt.Fprintf(r.w, "Window:      %s to %s\n",
		meta.WindowStart.Format("2006-01-02"), meta.WindowEnd.Format("2006-01-02"))
	fmt.Fprintf(r.w, "%s\n\n", strings.Repeat("=", 60))

	if len(recs) == 0 {
		fmt.Fprintf(r.w, "No recommendations available.\n")
		return nil
	}

	// Column headers
	fmt.Fprintf(r.w, "%-4s %-30s %6s %7s %7s %6s %8s %s\n",
		"Rank", "Configuration", "Nodes", "CPU%%", "Mem%%", "Score", "$/month", "Notes")
	fmt.Fprintf(r.w, "%s\n", strings.Repeat("-", 100))

	for _, rec := range recs {
		sr := rec.SimulationResult
		label := sr.InstanceConfig.Label()
		if len(label) > 30 {
			label = label[:27] + "..."
		}

		notes := ""
		if rec.CostVsBaseline < 0 {
			notes = fmt.Sprintf("%.1f%% savings", -rec.CostVsBaseline)
		} else if rec.CostVsBaseline > 0 {
			notes = fmt.Sprintf("+%.1f%% cost", rec.CostVsBaseline)
		}
		if len(sr.UnschedulablePods) > 0 {
			notes += fmt.Sprintf(" [%d unschedulable]", len(sr.UnschedulablePods))
		}

		fmt.Fprintf(r.w, "#%-3d %-30s %6d %6.1f%% %6.1f%% %6.1f %8.0f %s\n",
			rec.Rank,
			label,
			sr.TotalNodes,
			sr.AvgCPUUtilization*100,
			sr.AvgMemUtilization*100,
			rec.OverallScore,
			rec.MonthlyCost,
			notes,
		)
	}

	fmt.Fprintf(r.w, "%s\n", strings.Repeat("-", 100))

	// Top recommendation detail
	top := recs[0]
	topSR := top.SimulationResult
	fmt.Fprintf(r.w, "\nRecommended: %s\n", topSR.InstanceConfig.Label())
	fmt.Fprintf(r.w, "  Nodes:          %d\n", topSR.TotalNodes)
	fmt.Fprintf(r.w, "  Monthly cost:   $%.0f\n", top.MonthlyCost)
	fmt.Fprintf(r.w, "  CPU util:       %.1f%%\n", topSR.AvgCPUUtilization*100)
	fmt.Fprintf(r.w, "  Memory util:    %.1f%%\n", topSR.AvgMemUtilization*100)
	fmt.Fprintf(r.w, "  Balance score:  %.2f\n", topSR.Fragmentation.ResourceBalanceScore)

	if top.AnnualSavings > 0 {
		fmt.Fprintf(r.w, "  Annual savings: $%.0f\n", top.AnnualSavings)
	}

	if len(top.Warnings) > 0 {
		fmt.Fprintf(r.w, "\n  Warnings:\n")
		for _, w := range top.Warnings {
			fmt.Fprintf(r.w, "    - %s\n", w)
		}
	}

	fmt.Fprintf(r.w, "\n")
	return nil
}
