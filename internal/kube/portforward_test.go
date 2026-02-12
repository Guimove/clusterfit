package kube

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/fake"
)

func TestFindPodForService(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thanos-query",
				Namespace: "monitoring",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "thanos-query"},
				Ports:    []corev1.ServicePort{tcpPort("http", 9090)},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thanos-query-abc",
				Namespace: "monitoring",
				Labels:    map[string]string{"app": "thanos-query"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	podName, err := FindPodForService(context.Background(), client, "thanos-query", "monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if podName != "thanos-query-abc" {
		t.Errorf("expected pod thanos-query-abc, got %s", podName)
	}
}

func TestFindPodForServiceNoPods(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "thanos-query",
				Namespace: "monitoring",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "thanos-query"},
				Ports:    []corev1.ServicePort{tcpPort("http", 9090)},
			},
		},
	)

	_, err := FindPodForService(context.Background(), client, "thanos-query", "monitoring")
	if err == nil {
		t.Fatal("expected error when no running pods exist")
	}
}

func TestFindPodForServiceNoSelector(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "external-svc",
				Namespace: "monitoring",
			},
			Spec: corev1.ServiceSpec{
				Ports: []corev1.ServicePort{tcpPort("http", 9090)},
			},
		},
	)

	_, err := FindPodForService(context.Background(), client, "external-svc", "monitoring")
	if err == nil {
		t.Fatal("expected error when service has no selector")
	}
}

func TestFindPodForServiceSkipsPending(t *testing.T) {
	client := fake.NewSimpleClientset(
		&corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus",
				Namespace: "monitoring",
			},
			Spec: corev1.ServiceSpec{
				Selector: map[string]string{"app": "prometheus"},
				Ports:    []corev1.ServicePort{tcpPort("http", 9090)},
			},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus-pending",
				Namespace: "monitoring",
				Labels:    map[string]string{"app": "prometheus"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodPending},
		},
		&corev1.Pod{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "prometheus-running",
				Namespace: "monitoring",
				Labels:    map[string]string{"app": "prometheus"},
			},
			Status: corev1.PodStatus{Phase: corev1.PodRunning},
		},
	)

	podName, err := FindPodForService(context.Background(), client, "prometheus", "monitoring")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if podName != "prometheus-running" {
		t.Errorf("expected prometheus-running, got %s", podName)
	}
}
