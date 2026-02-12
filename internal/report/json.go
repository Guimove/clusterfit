package report

import (
	"context"
	"encoding/json"
	"fmt"
	"io"

	"github.com/guimove/clusterfit/internal/model"
)

// JSONReporter outputs recommendations as JSON.
type JSONReporter struct {
	w io.Writer
}

type jsonOutput struct {
	Meta            ReportMeta             `json:"meta"`
	Recommendations []model.Recommendation `json:"recommendations"`
}

func (r *JSONReporter) Report(ctx context.Context, recs []model.Recommendation, meta ReportMeta) error {
	output := jsonOutput{
		Meta:            meta,
		Recommendations: recs,
	}

	enc := json.NewEncoder(r.w)
	enc.SetIndent("", "  ")
	if err := enc.Encode(output); err != nil {
		return fmt.Errorf("encoding JSON output: %w", err)
	}
	return nil
}
