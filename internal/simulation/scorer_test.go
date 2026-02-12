package simulation

import (
	"testing"

	"github.com/guimove/clusterfit/internal/model"
)

func makeSimResult(cost float64, cpuUtil, memUtil float64, nodes int) model.SimulationResult {
	return model.SimulationResult{
		InstanceConfig: model.InstanceConfig{
			InstanceTypes: []model.NodeTemplate{{InstanceType: "m5.xlarge"}},
			Strategy:      "homogeneous",
		},
		TotalNodes:        nodes,
		TotalCost:         cost,
		AvgCPUUtilization: cpuUtil,
		AvgMemUtilization: memUtil,
		Fragmentation: model.FragmentationReport{
			ResourceBalanceScore: 0.8,
		},
	}
}

func TestScorer_CheapestWins(t *testing.T) {
	scorer := NewScorer(model.ScoringWeights{
		Cost: 1.0, Utilization: 0, Fragmentation: 0, Resilience: 0,
	})

	results := []model.SimulationResult{
		makeSimResult(1000, 0.5, 0.5, 10),
		makeSimResult(500, 0.5, 0.5, 10),
		makeSimResult(800, 0.5, 0.5, 10),
	}

	recs := scorer.RankResults(results, nil)

	if recs[0].MonthlyCost != 500 {
		t.Errorf("cheapest should rank first, got cost=%v", recs[0].MonthlyCost)
	}
	if recs[0].Rank != 1 {
		t.Errorf("expected rank 1, got %d", recs[0].Rank)
	}
}

func TestScorer_UtilizationMatters(t *testing.T) {
	scorer := NewScorer(model.ScoringWeights{
		Cost: 0, Utilization: 1.0, Fragmentation: 0, Resilience: 0,
	})

	results := []model.SimulationResult{
		makeSimResult(1000, 0.5, 0.5, 10),  // 50% avg
		makeSimResult(1000, 0.8, 0.9, 10),  // 85% avg
		makeSimResult(1000, 0.6, 0.7, 10),  // 65% avg
	}

	recs := scorer.RankResults(results, nil)

	if recs[0].SimulationResult.AvgCPUUtilization != 0.8 {
		t.Errorf("highest utilization should rank first, got CPU=%v",
			recs[0].SimulationResult.AvgCPUUtilization)
	}
}

func TestScorer_BaselineComparison(t *testing.T) {
	scorer := NewScorer(model.DefaultScoringWeights())

	baseline := makeSimResult(1000, 0.5, 0.5, 10)
	results := []model.SimulationResult{
		makeSimResult(800, 0.6, 0.6, 8),
		makeSimResult(1200, 0.4, 0.4, 12),
	}

	recs := scorer.RankResults(results, &baseline)

	// First result: $800 vs $1000 baseline = -20%
	if recs[0].CostVsBaseline > 0 && recs[1].CostVsBaseline > 0 {
		t.Error("expected at least one result with savings vs baseline")
	}

	// Check annual savings is set
	for _, rec := range recs {
		if rec.AnnualSavings == 0 {
			t.Error("expected non-zero annual savings when baseline is provided")
		}
	}
}

func TestScorer_ScoresInRange(t *testing.T) {
	scorer := NewScorer(model.DefaultScoringWeights())

	results := []model.SimulationResult{
		makeSimResult(500, 0.3, 0.4, 5),
		makeSimResult(1000, 0.7, 0.8, 20),
		makeSimResult(2000, 0.9, 0.9, 50),
	}

	recs := scorer.RankResults(results, nil)

	for _, rec := range recs {
		if rec.OverallScore < 0 || rec.OverallScore > 100 {
			t.Errorf("OverallScore out of range: %v", rec.OverallScore)
		}
		if rec.CostScore < 0 || rec.CostScore > 100 {
			t.Errorf("CostScore out of range: %v", rec.CostScore)
		}
		if rec.UtilizationScore < 0 || rec.UtilizationScore > 100 {
			t.Errorf("UtilizationScore out of range: %v", rec.UtilizationScore)
		}
	}
}

func TestScorer_Warnings(t *testing.T) {
	scorer := NewScorer(model.DefaultScoringWeights())

	// Result with unschedulable pods
	r := makeSimResult(500, 0.5, 0.5, 5)
	r.UnschedulablePods = []model.WorkloadProfile{makeWorkload("big", 8000, 32*1024*1024*1024)}

	recs := scorer.RankResults([]model.SimulationResult{r}, nil)
	if len(recs[0].Warnings) == 0 {
		t.Error("expected warnings for unschedulable pods")
	}

	// High CPU utilization
	r2 := makeSimResult(500, 0.90, 0.5, 5)
	recs2 := scorer.RankResults([]model.SimulationResult{r2}, nil)
	found := false
	for _, w := range recs2[0].Warnings {
		if w != "" {
			found = true
		}
	}
	if !found {
		t.Error("expected warning for high CPU utilization")
	}
}

func TestScorer_EmptyResults(t *testing.T) {
	scorer := NewScorer(model.DefaultScoringWeights())
	recs := scorer.RankResults(nil, nil)
	if recs != nil {
		t.Errorf("expected nil for empty results, got %v", recs)
	}
}
