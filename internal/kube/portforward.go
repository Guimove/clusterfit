package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"
)

// PortForwardSession represents an active port-forward tunnel to a pod.
type PortForwardSession struct {
	LocalPort int32
	stopChan  chan struct{}
}

// Close terminates the port-forward tunnel.
func (s *PortForwardSession) Close() {
	close(s.stopChan)
}

// FindPodForService resolves a service to one of its backing pods by reading the
// service's selector and finding a Running pod that matches.
func FindPodForService(ctx context.Context, client kubernetes.Interface, svcName, namespace string) (string, error) {
	svc, err := client.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return "", fmt.Errorf("getting service %s/%s: %w", namespace, svcName, err)
	}

	selector := svc.Spec.Selector
	if len(selector) == 0 {
		return "", fmt.Errorf("service %s/%s has no pod selector", namespace, svcName)
	}

	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: selector})
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return "", fmt.Errorf("listing pods for service %s/%s: %w", namespace, svcName, err)
	}

	for _, pod := range pods.Items {
		if pod.Status.Phase == corev1.PodRunning {
			return pod.Name, nil
		}
	}

	return "", fmt.Errorf("no running pod found for service %s/%s", namespace, svcName)
}

// StartPortForward opens a port-forward tunnel to the given pod and port.
// It binds to a random local port on 127.0.0.1 and returns a session that can
// be closed to stop the tunnel.
func StartPortForward(restConfig *rest.Config, client kubernetes.Interface, podName, namespace string, podPort int32) (*PortForwardSession, error) {
	transport, upgrader, err := spdy.RoundTripperFor(restConfig)
	if err != nil {
		return nil, fmt.Errorf("creating SPDY round-tripper: %w", err)
	}

	restClient := client.CoreV1().RESTClient()
	reqURL := restClient.Post().
		Resource("pods").
		Namespace(namespace).
		Name(podName).
		SubResource("portforward").
		URL()

	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, reqURL)

	stopChan := make(chan struct{}, 1)
	readyChan := make(chan struct{})

	// Use port 0 to let the OS assign a random local port
	ports := []string{fmt.Sprintf("0:%d", podPort)}
	fw, err := portforward.New(dialer, ports, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

	// Wait for the tunnel to be ready or fail
	select {
	case <-readyChan:
	case err := <-errChan:
		return nil, fmt.Errorf("port-forward failed: %w", err)
	}

	forwardedPorts, err := fw.GetPorts()
	if err != nil {
		close(stopChan)
		return nil, fmt.Errorf("getting forwarded ports: %w", err)
	}

	return &PortForwardSession{
		LocalPort: int32(forwardedPorts[0].Local),
		stopChan:  stopChan,
	}, nil
}

// PortForwardURL returns the SPDY port-forward URL for a pod. Exported for testing.
func PortForwardURL(host, namespace, podName string) *url.URL {
	return &url.URL{
		Scheme: "https",
		Host:   host,
		Path:   fmt.Sprintf("/api/v1/namespaces/%s/pods/%s/portforward", namespace, podName),
	}
}
