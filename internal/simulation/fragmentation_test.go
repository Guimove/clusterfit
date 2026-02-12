package simulation

import (
	"math"
	"testing"

	"github.com/guimove/clusterfit/internal/model"
)

func makeNodeAlloc(cpuMillis, memBytes, usedCPU, usedMem int64) model.NodeAllocation {
	return model.NodeAllocation{
		Template: model.NodeTemplate{
			AllocatableCPUMillis:   cpuMillis,
			AllocatableMemoryBytes: memBytes,
		},
		UsedCPU: usedCPU,
		UsedMem: usedMem,
	}
}

func memGiB(n int64) int64 { return n * 1024 * 1024 * 1024 }

func TestFragmentation_PerfectBalance(t *testing.T) {
	mem80pct := memGiB(16) * 80 / 100
	nodes := []model.NodeAllocation{
		makeNodeAlloc(4000, memGiB(16), 3200, mem80pct),
		makeNodeAlloc(4000, memGiB(16), 3200, mem80pct),
	}

	report := AnalyzeFragmentation(nodes)

	// 80% CPU, 80% memory → perfect balance
	if report.ResourceBalanceScore < 0.95 {
		t.Errorf("expected high balance score, got %v", report.ResourceBalanceScore)
	}
	if report.StrandedCPUMillis != 0 {
		t.Errorf("expected no stranded CPU, got %d", report.StrandedCPUMillis)
	}
	if report.StrandedMemoryBytes != 0 {
		t.Errorf("expected no stranded memory, got %d", report.StrandedMemoryBytes)
	}
}

func TestFragmentation_StrandedMemory(t *testing.T) {
	// 95% CPU, 20% memory → memory is stranded
	nodes := []model.NodeAllocation{
		makeNodeAlloc(4000, memGiB(16), 3800, memGiB(16)*20/100),
	}

	report := AnalyzeFragmentation(nodes)
	if report.StrandedMemoryBytes == 0 {
		t.Error("expected stranded memory")
	}
}

func TestFragmentation_StrandedCPU(t *testing.T) {
	// 20% CPU, 95% memory → CPU is stranded
	nodes := []model.NodeAllocation{
		makeNodeAlloc(4000, memGiB(16), 800, memGiB(16)*95/100),
	}

	report := AnalyzeFragmentation(nodes)
	if report.StrandedCPUMillis == 0 {
		t.Error("expected stranded CPU")
	}
}

func TestFragmentation_Underutilized(t *testing.T) {
	nodes := []model.NodeAllocation{
		makeNodeAlloc(4000, memGiB(16), 1000, memGiB(4)),  // 25% CPU, 25% mem
		makeNodeAlloc(4000, memGiB(16), 3200, memGiB(14)), // 80% CPU, 87% mem
	}

	report := AnalyzeFragmentation(nodes)
	if report.UnderutilizedNodeFraction != 0.5 {
		t.Errorf("expected 50%% underutilized, got %v", report.UnderutilizedNodeFraction)
	}
}

func TestFragmentation_EmptyNodes(t *testing.T) {
	report := AnalyzeFragmentation(nil)
	if report.ResourceBalanceScore != 1.0 {
		t.Errorf("expected 1.0 balance for empty, got %v", report.ResourceBalanceScore)
	}
}

func TestFragmentation_LowBalance(t *testing.T) {
	// Very imbalanced: 90% CPU, 10% memory
	nodes := []model.NodeAllocation{
		makeNodeAlloc(4000, memGiB(16), 3600, memGiB(16)*10/100),
	}

	report := AnalyzeFragmentation(nodes)
	// |0.9 - 0.1| = 0.8 → balance = 1.0 - 0.8 = 0.2
	if math.Abs(report.ResourceBalanceScore-0.2) > 0.05 {
		t.Errorf("expected low balance ~0.2, got %v", report.ResourceBalanceScore)
	}
}
