package kube

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
)

// DiscoveryResult holds the discovered metrics endpoint.
type DiscoveryResult struct {
	URL         string
	Type        string // "prometheus", "thanos", "cortex", "victoria-metrics", "mimir"
	ServiceName string
	Namespace   string
}

// DiscoveryOptions configures the service discovery search.
type DiscoveryOptions struct {
	Namespace string // empty = search all namespaces
}

// candidate describes a metrics backend to search for.
type candidate struct {
	backendType string
	selectors   []string
}

var candidates = []candidate{
	{
		backendType: "thanos",
		selectors: []string{
			"app.kubernetes.io/component=query,app.kubernetes.io/name=thanos",
			"app.kubernetes.io/name=thanos-query",
			"app=thanos-query",
			"app=thanos-querier",
		},
	},
	{
		backendType: "victoria-metrics",
		selectors: []string{
			"app.kubernetes.io/name=vmsingle",
			"app.kubernetes.io/name=victoria-metrics-single",
			"app.kubernetes.io/name=vmselect",
			"app=vmselect",
		},
	},
	{
		backendType: "mimir",
		selectors: []string{
			"app.kubernetes.io/name=mimir,app.kubernetes.io/component=query-frontend",
		},
	},
	{
		backendType: "cortex",
		selectors: []string{
			"app.kubernetes.io/name=cortex,app.kubernetes.io/component=query-frontend",
		},
	},
	{
		backendType: "prometheus",
		selectors: []string{
			"app=kube-prometheus-stack-prometheus",
			"app=prometheus,component=server",
			"app=prometheus-server",
			"app=prometheus-operator-prometheus",
			"app=prometheus-prometheus",
			"app.kubernetes.io/name=prometheus",
		},
	},
}

// Discover searches the Kubernetes cluster for a Prometheus-compatible metrics service.
// It tries well-known label selectors in priority order (Thanos, VictoriaMetrics, Mimir,
// Cortex, Prometheus) and returns the first match.
func Discover(ctx context.Context, client kubernetes.Interface, opts DiscoveryOptions) (*DiscoveryResult, error) {
	namespace := opts.Namespace
	if namespace == "" {
		namespace = "" // empty string = all namespaces in the k8s API
	}

	for _, c := range candidates {
		for _, selector := range c.selectors {
			svcList, err := client.CoreV1().Services(namespace).List(ctx, metav1.ListOptions{
				LabelSelector: selector,
			})
			if err != nil {
				continue
			}
			if len(svcList.Items) == 0 {
				continue
			}

			svc := svcList.Items[0]
			port := extractPort(svc)
			if port == 0 {
				continue
			}

			return &DiscoveryResult{
				URL:         fmt.Sprintf("http://%s.%s.svc:%d", svc.Name, svc.Namespace, port),
				Type:        c.backendType,
				ServiceName: svc.Name,
				Namespace:   svc.Namespace,
			}, nil
		}
	}

	return nil, fmt.Errorf("no Prometheus-compatible service found in the cluster; " +
		"use --prometheus-url to specify the endpoint manually")
}

// extractPort returns the best port from a Service, preferring well-known port names.
func extractPort(svc corev1.Service) int32 {
	preferredNames := map[string]bool{
		"http":     true,
		"web":      true,
		"http-web": true,
	}

	// First pass: look for a preferred port name
	for _, p := range svc.Spec.Ports {
		if preferredNames[p.Name] {
			return p.Port
		}
	}

	// Second pass: first TCP port
	for _, p := range svc.Spec.Ports {
		if p.Protocol == corev1.ProtocolTCP || p.Protocol == "" {
			return p.Port
		}
	}

	return 0
}
