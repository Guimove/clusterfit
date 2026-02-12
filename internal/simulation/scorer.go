package simulation

import (
	"fmt"
	"math"
	"sort"

	"github.com/guimove/clusterfit/internal/model"
)

// Scorer computes composite scores for simulation results and ranks them.
type Scorer struct {
	Weights model.ScoringWeights
}

// NewScorer creates a scorer with the given weights.
func NewScorer(weights model.ScoringWeights) *Scorer {
	return &Scorer{Weights: weights}
}

// RankResults scores and ranks a set of simulation results.
// If baseline is non-nil, cost comparisons are made against it.
func (s *Scorer) RankResults(results []model.SimulationResult, baseline *model.SimulationResult) []model.Recommendation {
	if len(results) == 0 {
		return nil
	}

	// Find cost bounds for normalization
	minCost, maxCost := results[0].TotalCost, results[0].TotalCost
	for _, r := range results[1:] {
		if r.TotalCost < minCost {
			minCost = r.TotalCost
		}
		if r.TotalCost > maxCost {
			maxCost = r.TotalCost
		}
	}

	recs := make([]model.Recommendation, len(results))
	for i, r := range results {
		recs[i] = s.score(r, baseline, minCost, maxCost)
	}

	// Sort by overall score descending
	sort.SliceStable(recs, func(i, j int) bool {
		return recs[i].OverallScore > recs[j].OverallScore
	})

	// Assign ranks
	for i := range recs {
		recs[i].Rank = i + 1
	}

	return recs
}

func (s *Scorer) score(
	r model.SimulationResult,
	baseline *model.SimulationResult,
	minCost, maxCost float64,
) model.Recommendation {
	rec := model.Recommendation{
		SimulationResult: r,
		MonthlyCost:      r.TotalCost,
	}

	// Cost score: 100 = cheapest, 0 = most expensive
	costRange := maxCost - minCost
	if costRange > 0 {
		rec.CostScore = (1.0 - (r.TotalCost-minCost)/costRange) * 100
	} else {
		rec.CostScore = 100
	}

	// Cost vs baseline
	if baseline != nil && baseline.TotalCost > 0 {
		rec.CostVsBaseline = ((r.TotalCost - baseline.TotalCost) / baseline.TotalCost) * 100
		rec.AnnualSavings = (baseline.TotalCost - r.TotalCost) * 12
	}

	// Utilization score: average of CPU and memory utilization (0-100)
	rec.UtilizationScore = ((r.AvgCPUUtilization + r.AvgMemUtilization) / 2.0) * 100

	// Fragmentation score: based on resource balance and underutilized nodes
	rec.FragmentationScore = r.Fragmentation.ResourceBalanceScore * 100 *
		(1.0 - r.Fragmentation.UnderutilizedNodeFraction)

	// Resilience score: penalize too few nodes (single point of failure)
	// and too many nodes (management overhead)
	switch {
	case r.TotalNodes <= 1:
		rec.ResilienceScore = 20
	case r.TotalNodes <= 3:
		rec.ResilienceScore = 60
	case r.TotalNodes <= 50:
		rec.ResilienceScore = 100
	case r.TotalNodes <= 200:
		rec.ResilienceScore = 80
	default:
		rec.ResilienceScore = 60
	}

	// Penalize unschedulable pods
	if len(r.UnschedulablePods) > 0 {
		penalty := math.Min(float64(len(r.UnschedulablePods))*10, 50)
		rec.ResilienceScore = math.Max(0, rec.ResilienceScore-penalty)
	}

	// Composite score
	rec.OverallScore = s.Weights.Cost*rec.CostScore +
		s.Weights.Utilization*rec.UtilizationScore +
		s.Weights.Fragmentation*rec.FragmentationScore +
		s.Weights.Resilience*rec.ResilienceScore

	// Generate rationale
	rec.Rationale = generateRationale(rec)

	// Generate warnings
	rec.Warnings = generateWarnings(r)

	return rec
}

func generateRationale(rec model.Recommendation) string {
	r := rec.SimulationResult
	label := r.InstanceConfig.Label()

	rationale := fmt.Sprintf("%s: %d nodes, $%.0f/mo, CPU %.0f%%, Mem %.0f%%",
		label, r.TotalNodes, r.TotalCost,
		r.AvgCPUUtilization*100, r.AvgMemUtilization*100)

	if rec.CostVsBaseline < 0 {
		rationale += fmt.Sprintf(" (%.1f%% savings)", -rec.CostVsBaseline)
	}

	return rationale
}

func generateWarnings(r model.SimulationResult) []string {
	var warnings []string

	if len(r.UnschedulablePods) > 0 {
		warnings = append(warnings,
			fmt.Sprintf("%d pods could not be scheduled", len(r.UnschedulablePods)))
	}

	if r.AvgCPUUtilization > HighUtilThreshold {
		warnings = append(warnings, "High CPU utilization leaves little headroom for bursts")
	}
	if r.AvgMemUtilization > CriticalMemUtilThreshold {
		warnings = append(warnings, "High memory utilization risks OOM under load spikes")
	}

	if r.Fragmentation.UnderutilizedNodeFraction > LowUtilThreshold {
		warnings = append(warnings,
			fmt.Sprintf("%.0f%% of nodes are underutilized (<%d%% on one dimension)",
				r.Fragmentation.UnderutilizedNodeFraction*100, int(LowUtilThreshold*100)))
	}

	// Spot warnings
	if r.InstanceConfig.SpotRatio > HighSpotRatio {
		warnings = append(warnings, "High spot ratio increases interruption risk")
	}

	return warnings
}
