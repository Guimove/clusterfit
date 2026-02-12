package kube

import (
	"context"
	"fmt"
	"io"
	"net/http"

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
	PodName   string
	stopChan  chan struct{}
}

// Close terminates the port-forward tunnel.
func (s *PortForwardSession) Close() {
	close(s.stopChan)
}

// PortForwardToService sets up a port-forward tunnel to a pod backing the given service.
// It resolves the service's targetPort (which may be a named port like "http") by
// looking at the pod's container ports, then opens the SPDY tunnel.
func PortForwardToService(ctx context.Context, restConfig *rest.Config, client kubernetes.Interface, svcName, namespace string, svcPort int32) (*PortForwardSession, error) {
	// Get the service to read selector and targetPort
	svc, err := client.CoreV1().Services(namespace).Get(ctx, svcName, metav1.GetOptions{})
	if err != nil {
		return nil, fmt.Errorf("getting service %s/%s: %w", namespace, svcName, err)
	}

	selector := svc.Spec.Selector
	if len(selector) == 0 {
		return nil, fmt.Errorf("service %s/%s has no pod selector", namespace, svcName)
	}

	// Find the ServicePort matching the discovered port
	var matchedPort *corev1.ServicePort
	for i := range svc.Spec.Ports {
		if svc.Spec.Ports[i].Port == svcPort {
			matchedPort = &svc.Spec.Ports[i]
			break
		}
	}
	if matchedPort == nil {
		return nil, fmt.Errorf("service %s/%s has no port %d", namespace, svcName, svcPort)
	}

	// Find a running pod
	labelSelector := metav1.FormatLabelSelector(&metav1.LabelSelector{MatchLabels: selector})
	pods, err := client.CoreV1().Pods(namespace).List(ctx, metav1.ListOptions{
		LabelSelector: labelSelector,
	})
	if err != nil {
		return nil, fmt.Errorf("listing pods for service %s/%s: %w", namespace, svcName, err)
	}

	var pod *corev1.Pod
	for i := range pods.Items {
		if pods.Items[i].Status.Phase == corev1.PodRunning {
			pod = &pods.Items[i]
			break
		}
	}
	if pod == nil {
		return nil, fmt.Errorf("no running pod found for service %s/%s", namespace, svcName)
	}

	// Resolve the target port (handles int, named string, or unset)
	containerPort := resolveTargetPort(*matchedPort, pod)

	// Start the tunnel
	session, err := startPortForward(restConfig, client, pod.Name, namespace, containerPort)
	if err != nil {
		return nil, err
	}
	session.PodName = pod.Name
	return session, nil
}

// resolveTargetPort resolves a ServicePort's targetPort to a numeric port.
// It handles three cases:
//  1. Integer targetPort → use directly
//  2. Named targetPort (e.g. "http") → look up in pod's container ports
//  3. Unset targetPort → defaults to the service port
func resolveTargetPort(sp corev1.ServicePort, pod *corev1.Pod) int32 {
	tp := sp.TargetPort

	// Case 1: numeric targetPort
	if tp.IntValue() != 0 {
		return int32(tp.IntValue())
	}

	// Case 2: named targetPort — resolve from pod containers
	if name := tp.String(); name != "" && name != "0" {
		for _, c := range pod.Spec.Containers {
			for _, cp := range c.Ports {
				if cp.Name == name {
					return cp.ContainerPort
				}
			}
		}
	}

	// Case 3: unset — defaults to service port
	return sp.Port
}

// startPortForward opens a port-forward tunnel to the given pod and port.
func startPortForward(restConfig *rest.Config, client kubernetes.Interface, podName, namespace string, podPort int32) (*PortForwardSession, error) {
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

	ports := []string{fmt.Sprintf("0:%d", podPort)}
	fw, err := portforward.New(dialer, ports, stopChan, readyChan, io.Discard, io.Discard)
	if err != nil {
		return nil, fmt.Errorf("creating port-forwarder: %w", err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- fw.ForwardPorts()
	}()

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
