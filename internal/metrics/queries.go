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

// queryRunningPods returns PromQL for currently running pods.
// The pod inventory is an instant snapshot, but per-pod P95/P99 CPU and memory
// metrics still cover the full quantile_over_time window (e.g. 7 days).
func queryRunningPods() string {
	return `kube_pod_status_phase{phase="Running"} == 1`
}

// queryPodOwner returns PromQL for pod owner references.
func queryPodOwner() string {
	return `kube_pod_owner{}`
}

// queryClusterCPUPercentile returns PromQL for cluster-wide aggregate CPU usage
// at a given percentile over the full window. Captures scaling peaks that
// per-pod instant snapshots miss.
func queryClusterCPUPercentile(percentile float64, window, step string) string {
	return fmt.Sprintf(`quantile_over_time(%g,
  sum(
    rate(container_cpu_usage_seconds_total{
      container!="",
      container!="POD",
      image!=""
    }[5m])
  )[%s:%s]
)`, percentile, window, step)
}

// queryClusterMemoryPercentile returns PromQL for cluster-wide aggregate memory
// usage at a given percentile over the full window.
func queryClusterMemoryPercentile(percentile float64, window, step string) string {
	return fmt.Sprintf(`quantile_over_time(%g,
  sum(
    container_memory_working_set_bytes{
      container!="",
      container!="POD",
      image!=""
    }
  )[%s:%s]
)`, percentile, window, step)
}

// queryMinNodeCount returns PromQL for the minimum observed node count over the window.
func queryMinNodeCount(window, step string) string {
	return fmt.Sprintf(`min_over_time(count(kube_node_info)[%s:%s])`, window, step)
}

// queryMaxNodeCount returns PromQL for the maximum observed node count over the window.
func queryMaxNodeCount(window, step string) string {
	return fmt.Sprintf(`max_over_time(count(kube_node_info)[%s:%s])`, window, step)
}
