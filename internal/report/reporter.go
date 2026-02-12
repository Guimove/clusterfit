package report

import (
	"context"
	"io"
	"time"

	"github.com/guimove/clusterfit/internal/model"
)

// Reporter formats and writes recommendations to an output destination.
type Reporter interface {
	Report(ctx context.Context, recs []model.Recommendation, meta ReportMeta) error
}

// ReportMeta contains contextual metadata for the report.
type ReportMeta struct {
	ClusterName  string
	Region       string
	CollectedAt  time.Time
	WindowStart  time.Time
	WindowEnd    time.Time
	Percentile   float64
	TotalPods    int
	TotalDaemons int
	Strategy     string
	MinNodes     int

	// Cluster-wide aggregate metrics (nil if unavailable)
	AggregateMetrics *model.ClusterAggregateMetrics

	// Workload classification (populated when auto-detection is used)
	WorkloadClass string                  // e.g. "general-purpose"
	GiBPerVCPU    float64                 // aggregate ratio
	Alternatives  []model.AlternativeArch // architecture alternatives
}

// NewReporter creates a reporter for the given format writing to w.
func NewReporter(format string, w io.Writer) Reporter {
	switch format {
	case "json":
		return &JSONReporter{w: w}
	case "markdown":
		return &MarkdownReporter{w: w}
	default:
		return &TableReporter{w: w}
	}
}
