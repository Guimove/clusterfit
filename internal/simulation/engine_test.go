package simulation

import (
	"context"
	"testing"

	"github.com/guimove/clusterfit/internal/model"
)

func TestEngine_RunAll(t *testing.T) {
	packer := &BestFitDecreasing{}
	scorer := NewScorer(model.DefaultScoringWeights())
	engine := NewEngine(packer, scorer)

	state := model.ClusterState{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app-1", 500, 1*1024*1024*1024),
			makeWorkload("app-2", 300, 2*1024*1024*1024),
			makeWorkload("app-3", 800, 512*1024*1024),
		},
	}

	scenarios := []Scenario{
		{
			Name:          "m5.large",
			InstanceTypes: []model.NodeTemplate{makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096)},
			Strategy:      "homogeneous",
		},
		{
			Name:          "m5.xlarge",
			InstanceTypes: []model.NodeTemplate{makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192)},
			Strategy:      "homogeneous",
		},
	}

	recs, err := engine.RunAll(context.Background(), scenarios, state)
	if err != nil {
		t.Fatal(err)
	}

	if len(recs) != 2 {
		t.Fatalf("expected 2 recommendations, got %d", len(recs))
	}

	// Should be ranked
	if recs[0].Rank != 1 || recs[1].Rank != 2 {
		t.Errorf("expected ranks 1,2 — got %d,%d", recs[0].Rank, recs[1].Rank)
	}

	// First recommendation should have highest overall score
	if recs[0].OverallScore < recs[1].OverallScore {
		t.Errorf("rank 1 score (%v) should >= rank 2 score (%v)",
			recs[0].OverallScore, recs[1].OverallScore)
	}
}

func TestEngine_NoScenarios(t *testing.T) {
	packer := &BestFitDecreasing{}
	scorer := NewScorer(model.DefaultScoringWeights())
	engine := NewEngine(packer, scorer)

	_, err := engine.RunAll(context.Background(), nil, model.ClusterState{})
	if err == nil {
		t.Error("expected error for no scenarios")
	}
}

func TestEngine_ContextCancellation(t *testing.T) {
	packer := &BestFitDecreasing{}
	scorer := NewScorer(model.DefaultScoringWeights())
	engine := NewEngine(packer, scorer)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	var workloads []model.WorkloadProfile
	for i := 0; i < 100; i++ {
		workloads = append(workloads, makeWorkload("app", 500, 1*1024*1024*1024))
	}

	state := model.ClusterState{Workloads: workloads}
	scenarios := []Scenario{
		{
			Name:          "m5.large",
			InstanceTypes: []model.NodeTemplate{makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096)},
			Strategy:      "homogeneous",
		},
	}

	_, err := engine.RunAll(ctx, scenarios, state)
	// May or may not error depending on timing, but should not panic
	_ = err
}

func TestGenerateScenarios_Homogeneous(t *testing.T) {
	templates := []model.NodeTemplate{
		makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
	}

	scenarios := GenerateScenarios(templates, "homogeneous", 0.0)
	if len(scenarios) != 2 {
		t.Fatalf("expected 2 homogeneous scenarios, got %d", len(scenarios))
	}
	for _, s := range scenarios {
		if s.Strategy != "homogeneous" {
			t.Errorf("expected homogeneous strategy, got %s", s.Strategy)
		}
		if len(s.InstanceTypes) != 1 {
			t.Errorf("expected 1 instance type per scenario, got %d", len(s.InstanceTypes))
		}
	}
}

func TestGenerateScenarios_Mixed(t *testing.T) {
	templates := []model.NodeTemplate{
		{InstanceType: "m5.large", InstanceFamily: "m5", AllocatableCPUMillis: 2000, AllocatableMemoryBytes: 8 * 1024 * 1024 * 1024, MaxPods: 29, OnDemandPricePerHour: 0.096},
		{InstanceType: "m5.xlarge", InstanceFamily: "m5", AllocatableCPUMillis: 4000, AllocatableMemoryBytes: 16 * 1024 * 1024 * 1024, MaxPods: 58, OnDemandPricePerHour: 0.192},
		{InstanceType: "c5.large", InstanceFamily: "c5", AllocatableCPUMillis: 2000, AllocatableMemoryBytes: 4 * 1024 * 1024 * 1024, MaxPods: 29, OnDemandPricePerHour: 0.085},
	}

	scenarios := GenerateScenarios(templates, "mixed", 0.3)

	// Should have 1 mixed scenario for m5 family (2 sizes), c5 has only 1 size → skipped
	found := false
	for _, s := range scenarios {
		if s.Strategy == "mixed" && len(s.InstanceTypes) == 2 {
			found = true
			if s.SpotRatio != 0.3 {
				t.Errorf("expected spot ratio 0.3, got %v", s.SpotRatio)
			}
		}
	}
	if !found {
		t.Error("expected a mixed scenario for m5 family")
	}
}

func TestGenerateScenarios_Both(t *testing.T) {
	templates := []model.NodeTemplate{
		{InstanceType: "m5.large", InstanceFamily: "m5", AllocatableCPUMillis: 2000, AllocatableMemoryBytes: 8 * 1024 * 1024 * 1024, MaxPods: 29, OnDemandPricePerHour: 0.096},
		{InstanceType: "m5.xlarge", InstanceFamily: "m5", AllocatableCPUMillis: 4000, AllocatableMemoryBytes: 16 * 1024 * 1024 * 1024, MaxPods: 58, OnDemandPricePerHour: 0.192},
	}

	scenarios := GenerateScenarios(templates, "both", 0.0)

	var homo, mixed int
	for _, s := range scenarios {
		switch s.Strategy {
		case "homogeneous":
			homo++
		case "mixed":
			mixed++
		}
	}

	if homo != 2 {
		t.Errorf("expected 2 homogeneous scenarios, got %d", homo)
	}
	if mixed != 1 {
		t.Errorf("expected 1 mixed scenario, got %d", mixed)
	}
}
