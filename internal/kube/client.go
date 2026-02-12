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
func NewClient(kubeconfig, context string) (*kubernetes.Clientset, string, error) {
	config, currentContext, err := buildConfig(kubeconfig, context)
	if err != nil {
		return nil, "", fmt.Errorf("building kubernetes config: %w", err)
	}

	client, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, "", fmt.Errorf("creating kubernetes client: %w", err)
	}

	return client, currentContext, nil
}

func buildConfig(kubeconfig, context string) (*rest.Config, string, error) {
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
			return nil, "", err
		}
		currentContext := rawConfig.CurrentContext
		if context != "" {
			currentContext = context
		}

		restConfig, err := clientConfig.ClientConfig()
		if err != nil {
			return nil, "", err
		}
		return restConfig, currentContext, nil
	}

	// Fall back to in-cluster config
	restConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, "", fmt.Errorf("no kubeconfig found and not running in-cluster: %w", err)
	}
	return restConfig, "", nil
}
