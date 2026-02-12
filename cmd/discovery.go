package cmd

import (
	"context"
	"fmt"

	"github.com/guimove/clusterfit/internal/kube"
	"github.com/guimove/clusterfit/internal/metrics"
)

// resolveCollector creates a MetricsCollector by either using the explicit
// --prometheus-url or by auto-discovering a Prometheus-compatible service
// in the Kubernetes cluster.
func resolveCollector(ctx context.Context) (*metrics.PrometheusCollector, error) {
	// Explicit URL takes precedence
	if cfg.Prometheus.URL != "" {
		return metrics.NewPrometheusCollector(cfg.Prometheus.URL,
			metrics.WithTimeout(cfg.Prometheus.Timeout))
	}

	// Auto-discovery mode
	if cfg.Kubernetes.Enabled {
		client, kubeContext, err := kube.NewClient(cfg.Kubernetes.Kubeconfig, cfg.Kubernetes.Context)
		if err != nil {
			return nil, fmt.Errorf("connecting to Kubernetes: %w", err)
		}

		result, err := kube.Discover(ctx, client, kube.DiscoveryOptions{
			Namespace: cfg.Kubernetes.DiscoveryNamespace,
		})
		if err != nil {
			return nil, err
		}

		if verbose {
			fmt.Printf("Discovered %s at %s (service: %s/%s)\n",
				result.Type, result.URL, result.Namespace, result.ServiceName)
		}

		// Auto-detect cluster name from kube context if not set
		if cfg.Cluster.Name == "" && kubeContext != "" {
			cfg.Cluster.Name = kubeContext
		}

		return metrics.NewPrometheusCollector(result.URL,
			metrics.WithTimeout(cfg.Prometheus.Timeout))
	}

	return nil, fmt.Errorf("provide --prometheus-url or use --discover to auto-detect the metrics endpoint")
}
