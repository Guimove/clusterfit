package metrics

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/guimove/clusterfit/internal/model"
)

func TestStaticCollector_FromState(t *testing.T) {
	state := &model.ClusterState{
		Workloads: []model.WorkloadProfile{
			{Name: "app", Namespace: "default", EffectiveCPUMillis: 500, EffectiveMemoryBytes: 1024},
		},
	}

	collector := NewStaticCollectorFromState(state)

	if err := collector.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	if collector.BackendType() != "static" {
		t.Errorf("expected backend type 'static', got %q", collector.BackendType())
	}

	result, err := collector.Collect(context.Background(), CollectOptions{})
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(result.Workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(result.Workloads))
	}
}

func TestStaticCollector_FromFile(t *testing.T) {
	content := `{
		"workloads": [
			{"name": "web", "namespace": "prod", "effective_cpu_millis": 500, "effective_memory_bytes": 1073741824}
		],
		"daemon_sets": [
			{"name": "fluentbit", "namespace": "logging", "effective_cpu_millis": 100, "effective_memory_bytes": 268435456, "is_daemon_set": true}
		]
	}`

	dir := t.TempDir()
	path := filepath.Join(dir, "cluster.json")
	if err := os.WriteFile(path, []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	collector := NewStaticCollector(path)

	if err := collector.Ping(context.Background()); err != nil {
		t.Fatalf("Ping failed: %v", err)
	}

	result, err := collector.Collect(context.Background(), CollectOptions{})
	if err != nil {
		t.Fatalf("Collect failed: %v", err)
	}

	if len(result.Workloads) != 1 {
		t.Errorf("expected 1 workload, got %d", len(result.Workloads))
	}
	if len(result.DaemonSets) != 1 {
		t.Errorf("expected 1 daemon set, got %d", len(result.DaemonSets))
	}
}

func TestStaticCollector_FileNotFound(t *testing.T) {
	collector := NewStaticCollector("/nonexistent/file.json")

	if err := collector.Ping(context.Background()); err == nil {
		t.Error("expected error for missing file")
	}
}

func TestStaticCollector_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(path, []byte("not json"), 0644); err != nil {
		t.Fatal(err)
	}

	collector := NewStaticCollector(path)
	_, err := collector.Collect(context.Background(), CollectOptions{})
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestStaticCollector_EmptyWorkloads(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(path, []byte(`{"workloads":[], "daemon_sets":[]}`), 0644); err != nil {
		t.Fatal(err)
	}

	collector := NewStaticCollector(path)
	_, err := collector.Collect(context.Background(), CollectOptions{})
	if err == nil {
		t.Error("expected error for empty workloads")
	}
}
