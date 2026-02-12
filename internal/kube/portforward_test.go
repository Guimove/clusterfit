package kube

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestResolveTargetPortInt(t *testing.T) {
	sp := corev1.ServicePort{
		Port:       9090,
		TargetPort: intstr.FromInt32(10902),
	}
	pod := &corev1.Pod{}
	if got := resolveTargetPort(sp, pod); got != 10902 {
		t.Errorf("expected 10902, got %d", got)
	}
}

func TestResolveTargetPortNamed(t *testing.T) {
	sp := corev1.ServicePort{
		Port:       9090,
		TargetPort: intstr.FromString("http"),
	}
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Ports: []corev1.ContainerPort{
						{Name: "grpc", ContainerPort: 10901},
						{Name: "http", ContainerPort: 10902},
					},
				},
			},
		},
	}
	if got := resolveTargetPort(sp, pod); got != 10902 {
		t.Errorf("expected 10902, got %d", got)
	}
}

func TestResolveTargetPortNamedNotFound(t *testing.T) {
	sp := corev1.ServicePort{
		Port:       9090,
		TargetPort: intstr.FromString("unknown"),
	}
	pod := &corev1.Pod{
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Ports: []corev1.ContainerPort{
						{Name: "http", ContainerPort: 10902},
					},
				},
			},
		},
	}
	// Falls back to service port
	if got := resolveTargetPort(sp, pod); got != 9090 {
		t.Errorf("expected fallback to 9090, got %d", got)
	}
}

func TestResolveTargetPortUnset(t *testing.T) {
	sp := corev1.ServicePort{
		Port: 9090,
		// TargetPort is zero value
	}
	pod := &corev1.Pod{}
	// Defaults to service port
	if got := resolveTargetPort(sp, pod); got != 9090 {
		t.Errorf("expected 9090, got %d", got)
	}
}
