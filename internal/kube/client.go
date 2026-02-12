package kube

import (
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
)

// NewClient creates a Kubernetes clientset using the following resolution order:
// 1. Explicit kubeconfig path (--kubeconfig flag)
// 2. KUBECONFIG environment variable
// 3. In-cluster config (when running as a pod)
// 4. ~/.kube/config default
//
// It returns the clientset, the rest.Config (needed for port-forwarding), the
// resolved context name, a boolean indicating whether we are running in-cluster,
// and an error.
func NewClient(kubeconfig, context string) (*kubernetes.Clientset, *rest.Config, string, bool, error) {
	config, currentContext, inCluster, err := buildConfig(kubeconfig, context)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, "", false, fmt.Errorf("creating kubernetes client: %w", err)
	}

	return client, config, currentContext, inCluster, nil
}

func buildConfig(kubeconfig, context string) (*rest.Config, string, bool, error) {
	// Resolve kubeconfig path
	kubeconfigPath := kubeconfig
	if kubeconfigPath == "" {
		kubeconfigPath = os.Getenv("KUBECONFIG")
	}
	if kubeconfigPath == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			defaultPath := filepath.Join(home, ".kube", "config")
			if _, err := os.Stat(defaultPath); err == nil {
				kubeconfigPath = defaultPath
			}
		}
	}

	// If we found a kubeconfig file, use it
	if kubeconfigPath != "" {
		rules := &clientcmd.ClientConfigLoadingRules{ExplicitPath: kubeconfigPath}
		overrides := &clientcmd.ConfigOverrides{}
		if context != "" {
			overrides.CurrentContext = context
		}

		clientConfig := clientcmd.NewNonInteractiveDeferredLoadingClientConfig(rules, overrides)

		// Get the raw config to extract the current context name
		rawConfig, err := clientConfig.RawConfig()
		if err != nil {
			return nil, "", false, err
		}
		currentContext := rawConfig.CurrentContext
		if context != "" {
			currentContext = context
		}

		restConfig, err := clientConfig.ClientConfig()
		if err != nil {
			return nil, "", false, err
		}
		return restConfig, currentContext, false, nil
	}

	// Fall back to in-cluster config
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", false, fmt.Errorf("no kubeconfig found and not running in-cluster: %w", err)
	}
	return restConfig, "", true, nil
}
