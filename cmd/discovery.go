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
//
// When running outside the cluster (kubeconfig mode), it automatically sets up
// a port-forward tunnel to the discovered service. The returned cleanup function
// must be called to close the tunnel (it is nil when no tunnel was created).
func resolveCollector(ctx context.Context) (*metrics.PrometheusCollector, func(), error) {
	// Explicit URL takes precedence
	if cfg.Prometheus.URL != "" {
		c, err := metrics.NewPrometheusCollector(cfg.Prometheus.URL,
			metrics.WithTimeout(cfg.Prometheus.Timeout))
		return c, nil, err
	}

	// Auto-discovery mode
	if cfg.Kubernetes.Enabled {
		client, restConfig, kubeContext, inCluster, err := kube.NewClient(cfg.Kubernetes.Kubeconfig, cfg.Kubernetes.Context)
		if err != nil {
			return nil, nil, fmt.Errorf("connecting to Kubernetes: %w", err)
		}

		result, err := kube.Discover(ctx, client, kube.DiscoveryOptions{
			Namespace: cfg.Kubernetes.DiscoveryNamespace,
		})
		if err != nil {
			return nil, nil, err
		}

		if verbose {
			fmt.Printf("Discovered %s at %s (service: %s/%s)\n",
				result.Type, result.URL, result.Namespace, result.ServiceName)
		}

		// Auto-detect cluster name from kube context if not set
		if cfg.Cluster.Name == "" && kubeContext != "" {
			cfg.Cluster.Name = kubeContext
		}

		promURL := result.URL
		var cleanup func()

		if !inCluster {
			// Running from a laptop — service DNS won't resolve.
			// Port-forward to a backing pod automatically.
			podName, err := kube.FindPodForService(ctx, client, result.ServiceName, result.Namespace)
			if err != nil {
				return nil, nil, fmt.Errorf("finding pod for port-forward: %w", err)
			}

			session, err := kube.StartPortForward(restConfig, client, podName, result.Namespace, result.Port)
			if err != nil {
				return nil, nil, fmt.Errorf("starting port-forward: %w", err)
			}

			promURL = fmt.Sprintf("http://127.0.0.1:%d", session.LocalPort)
			cleanup = session.Close

			if verbose {
				fmt.Printf("Port-forwarding %s/%s (pod %s) → %s\n",
					result.Namespace, result.ServiceName, podName, promURL)
			}
		}

		c, err := metrics.NewPrometheusCollector(promURL,
			metrics.WithTimeout(cfg.Prometheus.Timeout))
		if err != nil {
			if cleanup != nil {
				cleanup()
			}
			return nil, nil, err
		}
		return c, cleanup, nil
	}

	return nil, nil, fmt.Errorf("provide --prometheus-url or use --discover to auto-detect the metrics endpoint")
}
