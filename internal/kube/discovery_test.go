package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func svc(name, namespace string, labels map[string]string, ports []corev1.ServicePort) *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: corev1.ServiceSpec{
			Ports: ports,
		},
	}
}

func tcpPort(name string, port int32) corev1.ServicePort {
	return corev1.ServicePort{
		Name:     name,
		Port:     port,
		Protocol: corev1.ProtocolTCP,
	}
}

func TestDiscoverThanos(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("thanos-query", "monitoring", map[string]string{
			"app.kubernetes.io/component": "query",
			"app.kubernetes.io/name":      "thanos",
		}, []corev1.ServicePort{tcpPort("http", 9090)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "thanos" {
		t.Errorf("expected type thanos, got %s", result.Type)
	}
	if result.URL != "http://thanos-query.monitoring.svc:9090" {
		t.Errorf("unexpected URL: %s", result.URL)
	}
	if result.Port != 9090 {
		t.Errorf("expected port 9090, got %d", result.Port)
	}
}

func TestDiscoverThanosSimpleLabel(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("thanos-querier", "observability", map[string]string{
			"app": "thanos-querier",
		}, []corev1.ServicePort{tcpPort("http", 10902)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "thanos" {
		t.Errorf("expected type thanos, got %s", result.Type)
	}
	if result.ServiceName != "thanos-querier" {
		t.Errorf("expected service name thanos-querier, got %s", result.ServiceName)
	}
	if result.Port != 10902 {
		t.Errorf("expected port 10902, got %d", result.Port)
	}
}

func TestDiscoverPrometheus(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("prometheus-server", "monitoring", map[string]string{
			"app": "prometheus-server",
		}, []corev1.ServicePort{tcpPort("http", 9090)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "prometheus" {
		t.Errorf("expected type prometheus, got %s", result.Type)
	}
	if result.URL != "http://prometheus-server.monitoring.svc:9090" {
		t.Errorf("unexpected URL: %s", result.URL)
	}
}

func TestDiscoverPrometheusKubeStack(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("kube-prometheus-stack-prometheus", "monitoring", map[string]string{
			"app": "kube-prometheus-stack-prometheus",
		}, []corev1.ServicePort{tcpPort("web", 9090)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "prometheus" {
		t.Errorf("expected type prometheus, got %s", result.Type)
	}
}

func TestDiscoverVictoriaMetrics(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("vmsingle", "vm", map[string]string{
			"app.kubernetes.io/name": "vmsingle",
		}, []corev1.ServicePort{tcpPort("http", 8428)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "victoria-metrics" {
		t.Errorf("expected type victoria-metrics, got %s", result.Type)
	}
	if result.URL != "http://vmsingle.vm.svc:8428" {
		t.Errorf("unexpected URL: %s", result.URL)
	}
	if result.Port != 8428 {
		t.Errorf("expected port 8428, got %d", result.Port)
	}
}

func TestDiscoverMimir(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("mimir-query-frontend", "mimir", map[string]string{
			"app.kubernetes.io/name":      "mimir",
			"app.kubernetes.io/component": "query-frontend",
		}, []corev1.ServicePort{tcpPort("http", 8080)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "mimir" {
		t.Errorf("expected type mimir, got %s", result.Type)
	}
}

func TestDiscoverCortex(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("cortex-query-frontend", "cortex", map[string]string{
			"app.kubernetes.io/name":      "cortex",
			"app.kubernetes.io/component": "query-frontend",
		}, []corev1.ServicePort{tcpPort("http", 8080)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "cortex" {
		t.Errorf("expected type cortex, got %s", result.Type)
	}
}

func TestDiscoverNamespaceFilter(t *testing.T) {
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("prometheus-server", "team-a", map[string]string{
			"app": "prometheus-server",
		}, []corev1.ServicePort{tcpPort("http", 9090)}),
		svc("prometheus-server", "team-b", map[string]string{
			"app": "prometheus-server",
		}, []corev1.ServicePort{tcpPort("http", 9090)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{Namespace: "team-b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Namespace != "team-b" {
		t.Errorf("expected namespace team-b, got %s", result.Namespace)
	}
}

func TestDiscoverNotFound(t *testing.T) {
	client := fake.NewSimpleClientset() //nolint:staticcheck // NewClientset requires generated apply configs

	_, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err == nil {
		t.Fatal("expected error when no service is found")
	}
}

func TestDiscoverPriorityOrder(t *testing.T) {
	// Both Thanos and Prometheus exist; Thanos should win.
	client := fake.NewSimpleClientset( //nolint:staticcheck // NewClientset requires generated apply configs
		svc("thanos-query", "monitoring", map[string]string{
			"app": "thanos-query",
		}, []corev1.ServicePort{tcpPort("http", 10902)}),
		svc("prometheus-server", "monitoring", map[string]string{
			"app": "prometheus-server",
		}, []corev1.ServicePort{tcpPort("http", 9090)}),
	)

	result, err := Discover(context.Background(), client, DiscoveryOptions{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Type != "thanos" {
		t.Errorf("expected thanos to take priority, got %s", result.Type)
	}
}

func TestExtractPortPreference(t *testing.T) {
	// Port named "http" should be preferred over unnamed port
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: 10901, Protocol: corev1.ProtocolTCP},
				{Name: "http", Port: 9090, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	port := extractPort(svc)
	if port != 9090 {
		t.Errorf("expected port 9090, got %d", port)
	}
}

func TestExtractPortFallbackTCP(t *testing.T) {
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "metrics", Port: 8080, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	port := extractPort(svc)
	if port != 8080 {
		t.Errorf("expected port 8080, got %d", port)
	}
}

func TestExtractPortWebName(t *testing.T) {
	svc := corev1.Service{
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{Name: "grpc", Port: 10901, Protocol: corev1.ProtocolTCP},
				{Name: "web", Port: 9090, Protocol: corev1.ProtocolTCP},
			},
		},
	}
	port := extractPort(svc)
	if port != 9090 {
		t.Errorf("expected port 9090, got %d", port)
	}
}
