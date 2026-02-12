package simulation

import (
	"context"
	"testing"

	"github.com/guimove/clusterfit/internal/model"
)

// helper to create a simple node template
func makeTemplate(instanceType string, cpuMillis int64, memBytes int64, maxPods int32, pricePerHour float64) model.NodeTemplate {
	return model.NodeTemplate{
		InstanceType:           instanceType,
		InstanceFamily:         instanceType[:2],
		AllocatableCPUMillis:   cpuMillis,
		AllocatableMemoryBytes: memBytes,
		MaxPods:                maxPods,
		OnDemandPricePerHour:   pricePerHour,
		CapacityType:           model.CapacityOnDemand,
	}
}

// helper to create a workload
func makeWorkload(name string, cpuMillis int64, memBytes int64) model.WorkloadProfile {
	return model.WorkloadProfile{
		Name:                 name,
		Namespace:            "default",
		EffectiveCPUMillis:   cpuMillis,
		EffectiveMemoryBytes: memBytes,
	}
}

func TestBFD_SinglePodSingleNode(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("pod-1", 1000, 2*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}
	if len(result.UnschedulablePods) != 0 {
		t.Fatalf("expected 0 unschedulable, got %d", len(result.UnschedulablePods))
	}
	if result.Nodes[0].PodCount != 1 {
		t.Errorf("expected 1 pod on node, got %d", result.Nodes[0].PodCount)
	}
}

func TestBFD_TwoPodsTwoNodes(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("pod-1", 1500, 4*1024*1024*1024),
			makeWorkload("pod-2", 1500, 4*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes, got %d", len(result.Nodes))
	}
}

func TestBFD_ManySmallPods(t *testing.T) {
	packer := &BestFitDecreasing{}

	var workloads []model.WorkloadProfile
	for i := 0; i < 100; i++ {
		workloads = append(workloads, makeWorkload(
			"small-pod",
			100,                  // 0.1 vCPU
			128*1024*1024,        // 128 MiB
		))
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			// 4 vCPU, 16 GiB, max 58 pods
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UnschedulablePods) != 0 {
		t.Errorf("expected 0 unschedulable, got %d", len(result.UnschedulablePods))
	}

	// 100 pods × 0.1 CPU = 10 vCPU total → need 3 nodes (4 vCPU each)
	// But pod limit of 58 means at most 58 pods per node → need 2 nodes for pods
	// CPU: 10 vCPU / 4 vCPU per node = 3 nodes (CPU bound)
	if len(result.Nodes) < 2 {
		t.Errorf("expected at least 2 nodes, got %d", len(result.Nodes))
	}
}

func TestBFD_CPUBound(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("cpu-hog-1", 3000, 1*1024*1024*1024),
			makeWorkload("cpu-hog-2", 3000, 1*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	// Each pod needs 3 vCPU, node has 4 vCPU → 1 pod per node → 2 nodes
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (CPU bound), got %d", len(result.Nodes))
	}
}

func TestBFD_MemoryBound(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("mem-hog-1", 100, 12*1024*1024*1024),
			makeWorkload("mem-hog-2", 100, 12*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	// Each pod needs 12 GiB, node has 16 GiB → 1 pod per node → 2 nodes
	if len(result.Nodes) != 2 {
		t.Fatalf("expected 2 nodes (memory bound), got %d", len(result.Nodes))
	}
}

func TestBFD_UnschedulablePod(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("too-big", 8000, 32*1024*1024*1024), // 8 vCPU, 32 GiB
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096), // 2 vCPU, 8 GiB
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
	if len(result.UnschedulablePods) != 1 {
		t.Fatalf("expected 1 unschedulable, got %d", len(result.UnschedulablePods))
	}
}

func TestBFD_MaxPodsConstraint(t *testing.T) {
	packer := &BestFitDecreasing{}

	var workloads []model.WorkloadProfile
	for i := 0; i < 50; i++ {
		workloads = append(workloads, makeWorkload("tiny", 10, 1024*1024)) // 0.01 vCPU, 1 MiB
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			// High CPU/mem but only 20 pods max
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 20, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	// 50 pods / 20 max per node = 3 nodes
	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (pod limit), got %d", len(result.Nodes))
	}
}

func TestBFD_DaemonSetOverhead(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app", 1500, 4*1024*1024*1024),
		},
		DaemonSets: []model.WorkloadProfile{
			makeWorkload("fluentbit", 200, 256*1024*1024),
			makeWorkload("datadog", 300, 512*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}
	// DaemonSets: 500m CPU, 768 MiB → remaining: 1500m CPU, ~7.25 GiB
	// App: 1500m CPU → fits
}

func TestBFD_DaemonSetOverhead_ForcesLargerNode(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app", 1500, 6*1024*1024*1024),
		},
		DaemonSets: []model.WorkloadProfile{
			makeWorkload("fluentbit", 500, 3*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			// Small node: can't fit app + daemonset
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
			// Large node: can fit both
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node, got %d", len(result.Nodes))
	}
	// Should select xlarge because large doesn't have enough after daemonset overhead
	if result.Nodes[0].Template.InstanceType != "m5.xlarge" {
		t.Errorf("expected m5.xlarge, got %s", result.Nodes[0].Template.InstanceType)
	}
}

func TestBFD_EmptyWorkloads(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: nil,
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 0 {
		t.Errorf("expected 0 nodes, got %d", len(result.Nodes))
	}
}

func TestBFD_NoTemplates(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app", 1000, 2*1024*1024*1024),
		},
		NodeTemplates: nil,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UnschedulablePods) != 1 {
		t.Errorf("expected 1 unschedulable, got %d", len(result.UnschedulablePods))
	}
}

func TestBFD_SpotRatio(t *testing.T) {
	packer := &BestFitDecreasing{}

	var workloads []model.WorkloadProfile
	for i := 0; i < 10; i++ {
		workloads = append(workloads, makeWorkload("app", 500, 1*1024*1024*1024))
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
		SpotRatio: 0.7,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}

	var spotCount, odCount int
	for _, n := range result.Nodes {
		if n.Template.CapacityType == model.CapacitySpot {
			spotCount++
		} else {
			odCount++
		}
	}

	totalNodes := len(result.Nodes)
	expectedSpot := int(float64(totalNodes)*0.7 + 0.5)
	if spotCount != expectedSpot {
		t.Errorf("spot nodes: got %d, want %d (total %d)", spotCount, expectedSpot, totalNodes)
	}
}

func TestBFD_Deterministic(t *testing.T) {
	packer := &BestFitDecreasing{}

	workloads := []model.WorkloadProfile{
		makeWorkload("a", 500, 1*1024*1024*1024),
		makeWorkload("b", 300, 2*1024*1024*1024),
		makeWorkload("c", 800, 512*1024*1024),
		makeWorkload("d", 200, 3*1024*1024*1024),
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
	}

	r1, _ := packer.Pack(context.Background(), input)
	r2, _ := packer.Pack(context.Background(), input)

	if len(r1.Nodes) != len(r2.Nodes) {
		t.Fatalf("non-deterministic: %d vs %d nodes", len(r1.Nodes), len(r2.Nodes))
	}

	for i := range r1.Nodes {
		if r1.Nodes[i].PodCount != r2.Nodes[i].PodCount {
			t.Errorf("node %d: pod count differs: %d vs %d",
				i, r1.Nodes[i].PodCount, r2.Nodes[i].PodCount)
		}
	}
}

func TestBFD_MixedResourceProfiles(t *testing.T) {
	packer := &BestFitDecreasing{}
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("cpu-heavy", 3000, 512*1024*1024),
			makeWorkload("mem-heavy", 100, 14*1024*1024*1024),
			makeWorkload("balanced", 1500, 6*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		},
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.UnschedulablePods) != 0 {
		t.Errorf("expected 0 unschedulable, got %d", len(result.UnschedulablePods))
	}
	// cpu-heavy (3 vCPU) + mem-heavy (14 GiB) should fit on same node (3.1 vCPU, 14.5 GiB)
	// balanced needs its own node
	if len(result.Nodes) > 2 {
		t.Errorf("expected at most 2 nodes for efficient packing, got %d", len(result.Nodes))
	}
}

func TestBFD_MaxNodesConstraint(t *testing.T) {
	packer := &BestFitDecreasing{}

	var workloads []model.WorkloadProfile
	for i := 0; i < 20; i++ {
		workloads = append(workloads, makeWorkload("app", 1500, 6*1024*1024*1024))
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
		MaxNodes: 5,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) > 5 {
		t.Errorf("expected max 5 nodes, got %d", len(result.Nodes))
	}
	if len(result.UnschedulablePods) == 0 {
		t.Error("expected some unschedulable pods with max 5 nodes")
	}
}

func TestBFD_MinNodes_Padding(t *testing.T) {
	packer := &BestFitDecreasing{}
	// Single small workload that fits on 1 node, but MinNodes=3
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app", 500, 1*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
		MinNodes: 3,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 3 {
		t.Fatalf("expected 3 nodes (MinNodes padding), got %d", len(result.Nodes))
	}
	// First node should have the workload, other 2 should be empty
	if result.Nodes[0].PodCount != 1 {
		t.Errorf("expected 1 pod on first node, got %d", result.Nodes[0].PodCount)
	}
	if result.Nodes[1].PodCount != 0 {
		t.Errorf("expected 0 pods on padded node, got %d", result.Nodes[1].PodCount)
	}
}

func TestBFD_MinNodes_AlreadyMet(t *testing.T) {
	packer := &BestFitDecreasing{}
	// 5 workloads that need 5 nodes, MinNodes=3 → no extra padding
	var workloads []model.WorkloadProfile
	for i := 0; i < 5; i++ {
		workloads = append(workloads, makeWorkload("app", 1500, 6*1024*1024*1024))
	}

	input := PackInput{
		Workloads: workloads,
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
		MinNodes: 3,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 5 {
		t.Fatalf("expected 5 nodes (already > MinNodes), got %d", len(result.Nodes))
	}
}

func TestBFD_MinNodes_Zero(t *testing.T) {
	packer := &BestFitDecreasing{}
	// MinNodes=0 should not pad
	input := PackInput{
		Workloads: []model.WorkloadProfile{
			makeWorkload("app", 500, 1*1024*1024*1024),
		},
		NodeTemplates: []model.NodeTemplate{
			makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		},
		MinNodes: 0,
	}

	result, err := packer.Pack(context.Background(), input)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Nodes) != 1 {
		t.Fatalf("expected 1 node (no MinNodes padding), got %d", len(result.Nodes))
	}
}

func BenchmarkBFD_1500Pods(b *testing.B) {
	var workloads []model.WorkloadProfile
	for i := 0; i < 1500; i++ {
		// Vary workloads: some CPU-heavy, some memory-heavy
		cpu := int64(100 + (i%10)*200)
		mem := int64((256 + (i%8)*512) * 1024 * 1024)
		workloads = append(workloads, makeWorkload("pod", cpu, mem))
	}

	templates := []model.NodeTemplate{
		makeTemplate("m5.large", 2000, 8*1024*1024*1024, 29, 0.096),
		makeTemplate("m5.xlarge", 4000, 16*1024*1024*1024, 58, 0.192),
		makeTemplate("m5.2xlarge", 8000, 32*1024*1024*1024, 58, 0.384),
	}

	input := PackInput{
		Workloads:     workloads,
		NodeTemplates: templates,
	}

	packer := &BestFitDecreasing{}
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		_, _ = packer.Pack(context.Background(), input)
	}
}
