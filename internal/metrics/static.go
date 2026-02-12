package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"github.com/guimove/clusterfit/internal/model"
)

// StaticCollector loads workload profiles from a JSON file.
// Used for testing, offline analysis, and CI pipelines.
type StaticCollector struct {
	filePath string
	state    *model.ClusterState
}

// NewStaticCollector creates a collector that reads from a JSON file.
func NewStaticCollector(filePath string) *StaticCollector {
	return &StaticCollector{filePath: filePath}
}

// NewStaticCollectorFromState creates a collector from a pre-built ClusterState.
func NewStaticCollectorFromState(state *model.ClusterState) *StaticCollector {
	return &StaticCollector{state: state}
}

// Ping checks that the file exists and is valid JSON.
func (s *StaticCollector) Ping(ctx context.Context) error {
	if s.state != nil {
		return nil
	}
	_, err := os.Stat(s.filePath)
	if err != nil {
		return fmt.Errorf("static metrics file: %w", err)
	}
	return nil
}

// BackendType returns "static".
func (s *StaticCollector) BackendType() string {
	return "static"
}

// Collect loads the cluster state from the JSON file.
func (s *StaticCollector) Collect(ctx context.Context, opts CollectOptions) (*model.ClusterState, error) {
	if s.state != nil {
		return s.state, nil
	}

	data, err := os.ReadFile(s.filePath)
	if err != nil {
		return nil, fmt.Errorf("reading static metrics file: %w", err)
	}

	var state model.ClusterState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing static metrics file: %w", err)
	}

	if len(state.Workloads) == 0 && len(state.DaemonSets) == 0 {
		return nil, ErrNoMetricsFound
	}

	return &state, nil
}
