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

func (r *TableReporter) Report(_ context.Context, recs []model.Recommendation, meta ReportMeta) error {
	ew := &errWriter{w: r.w}

	// Header
	ew.printf("\n")
	ew.printf("ClusterFit Recommendations\n")
	ew.printf("%s\n", strings.Repeat("=", 60))
	ew.printf("Cluster:     %s\n", meta.ClusterName)
	ew.printf("Region:      %s\n", meta.Region)
	ew.printf("Pods:        %d (+ %d DaemonSets)\n", meta.TotalPods, meta.TotalDaemons)
	ew.printf("Percentile:  p%g\n", meta.Percentile*100)
	ew.printf("Window:      %s to %s\n",
		meta.WindowStart.Format("2006-01-02"), meta.WindowEnd.Format("2006-01-02"))
	ew.printf("%s\n\n", strings.Repeat("=", 60))

	if len(recs) == 0 {
		ew.printf("No recommendations available.\n")
		return ew.err
	}

	// Column headers
	ew.printf("%-4s %-30s %6s %7s %7s %6s %8s %s\n",
		"Rank", "Configuration", "Nodes", "CPU%%", "Mem%%", "Score", "$/month", "Notes")
	ew.printf("%s\n", strings.Repeat("-", 100))

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

		ew.printf("#%-3d %-30s %6d %6.1f%% %6.1f%% %6.1f %8.0f %s\n",
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

	ew.printf("%s\n", strings.Repeat("-", 100))

	// Top recommendation detail
	top := recs[0]
	topSR := top.SimulationResult
	ew.printf("\nRecommended: %s\n", topSR.InstanceConfig.Label())
	ew.printf("  Nodes:          %d\n", topSR.TotalNodes)
	ew.printf("  Monthly cost:   $%.0f\n", top.MonthlyCost)
	ew.printf("  CPU util:       %.1f%%\n", topSR.AvgCPUUtilization*100)
	ew.printf("  Memory util:    %.1f%%\n", topSR.AvgMemUtilization*100)
	ew.printf("  Balance score:  %.2f\n", topSR.Fragmentation.ResourceBalanceScore)

	if top.AnnualSavings > 0 {
		ew.printf("  Annual savings: $%.0f\n", top.AnnualSavings)
	}

	if len(top.Warnings) > 0 {
		ew.printf("\n  Warnings:\n")
		for _, w := range top.Warnings {
			ew.printf("    - %s\n", w)
		}
	}

	ew.printf("\n")
	return ew.err
}
