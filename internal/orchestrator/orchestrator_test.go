package orchestrator

import (
	"bytes"
	"context"
	"testing"

	"github.com/guimove/clusterfit/internal/config"
	"github.com/guimove/clusterfit/internal/metrics"
	"github.com/guimove/clusterfit/internal/model"
)

// mockProvider implements aws.PricingProvider for testing.
type mockProvider struct {
	templates []model.NodeTemplate
	region    string
}

func (m *mockProvider) GetInstanceTypes(ctx context.Context, filter interface{}) ([]model.NodeTemplate, error) {
	return m.templates, nil
}

func (m *mockProvider) GetSpotPrices(ctx context.Context, instanceTypes []string) (map[string]float64, error) {
	return nil, nil
}

func (m *mockProvider) Region() string { return m.region }

func TestOrchestrator_Simulate(t *testing.T) {
	state := &model.ClusterState{
		Workloads: []model.WorkloadProfile{
			{Name: "app-1", Namespace: "default", EffectiveCPUMillis: 500, EffectiveMemoryBytes: 1 * 1024 * 1024 * 1024},
			{Name: "app-2", Namespace: "default", EffectiveCPUMillis: 300, EffectiveMemoryBytes: 2 * 1024 * 1024 * 1024},
			{Name: "app-3", Namespace: "default", EffectiveCPUMillis: 800, EffectiveMemoryBytes: 512 * 1024 * 1024},
		},
	}

	templates := []model.NodeTemplate{
		{
			InstanceType:           "m5.large",
			InstanceFamily:         "m5",
			AllocatableCPUMillis:   1940,
			AllocatableMemoryBytes: 7 * 1024 * 1024 * 1024,
			MaxPods:                29,
			OnDemandPricePerHour:   0.096,
			CapacityType:           model.CapacityOnDemand,
		},
		{
			InstanceType:           "m5.xlarge",
			InstanceFamily:         "m5",
			AllocatableCPUMillis:   3920,
			AllocatableMemoryBytes: 15 * 1024 * 1024 * 1024,
			MaxPods:                58,
			OnDemandPricePerHour:   0.192,
			CapacityType:           model.CapacityOnDemand,
		},
	}

	cfg := config.Default()
	cfg.Simulation.Strategy = "both"
	cfg.Output.TopN = 3

	collector := metrics.NewStaticCollectorFromState(state)
	orch := &Orchestrator{
		Collector: collector,
		Config:    cfg,
		Writer:    &bytes.Buffer{},
	}

	recs, err := orch.Simulate(context.Background(), state, templates)
	if err != nil {
		t.Fatalf("Simulate failed: %v", err)
	}

	if len(recs) == 0 {
		t.Fatal("expected at least 1 recommendation")
	}

	// Verify recs are ranked
	for i := 0; i < len(recs)-1; i++ {
		if recs[i].OverallScore < recs[i+1].OverallScore {
			t.Errorf("recommendations not sorted: score[%d]=%v < score[%d]=%v",
				i, recs[i].OverallScore, i+1, recs[i+1].OverallScore)
		}
	}
}
