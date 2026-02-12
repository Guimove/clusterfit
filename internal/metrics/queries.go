package metrics

import "fmt"

// PromQL query templates for collecting pod resource metrics.
//
// These queries are designed to work with:
//   - Standard Prometheus + cAdvisor metrics (container_cpu_usage_seconds_total, container_memory_working_set_bytes)
//   - kube-state-metrics (kube_pod_container_resource_requests, kube_pod_owner)
//   - Both Prometheus and Thanos/Cortex backends

// queryCPUPercentile returns PromQL for CPU usage at a given percentile over a time range.
// Returns CPU in cores per (namespace, pod).
func queryCPUPercentile(percentile float64, window, step string) string {
	return fmt.Sprintf(`quantile_over_time(%g,
  sum by (namespace, pod) (
    rate(container_cpu_usage_seconds_total{
      container!="",
      container!="POD",
      image!=""
    }[5m])
  )[%s:%s]
)`, percentile, window, step)
}

// queryMemoryPercentile returns PromQL for memory usage at a given percentile.
// Returns memory in bytes per (namespace, pod).
func queryMemoryPercentile(percentile float64, window, step string) string {
	return fmt.Sprintf(`quantile_over_time(%g,
  sum by (namespace, pod) (
    container_memory_working_set_bytes{
      container!="",
      container!="POD",
      image!=""
    }
  )[%s:%s]
)`, percentile, window, step)
}

// queryPodResourceRequests returns PromQL for pod resource requests.
func queryPodResourceRequests(resource string) string {
	return fmt.Sprintf(`sum by (namespace, pod) (
  kube_pod_container_resource_requests{resource="%s"}
)`, resource)
}

// queryPodResourceLimits returns PromQL for pod resource limits.
func queryPodResourceLimits(resource string) string {
	return fmt.Sprintf(`sum by (namespace, pod) (
  kube_pod_container_resource_limits{resource="%s"}
)`, resource)
}

// queryPodOwner returns PromQL for pod owner references.
func queryPodOwner() string {
	return `kube_pod_owner{}`
}
