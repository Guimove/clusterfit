# ClusterFit

EC2 instance sizing recommender for EKS clusters. ClusterFit analyzes real pod resource usage from Prometheus, runs bin-packing simulations across instance types, and ranks them by cost, utilization, and fragmentation.

Think of it as [Goldilocks](https://github.com/FairwindsOps/goldilocks) for **nodes** instead of pods — finding instance types that are *just right* for your workload mix.

## Features

- **Metrics-driven** — Collects actual CPU/memory usage from Prometheus, Thanos, Cortex, Victoria Metrics, or Mimir
- **Scaling-aware** — Cluster-level P95 CPU/memory over the full window and observed min/max node counts capture HPA/autoscaler peaks that point-in-time snapshots miss
- **HA constraint** — Enforces a minimum node count (default 3) so recommendations never drop below your availability floor
- **Auto-discovery** — Finds your metrics endpoint in Kubernetes automatically (KRR-style `--discover` flag)
- **Bin-packing simulation** — Best Fit Decreasing algorithm packs your workloads into candidate instance types
- **Multi-strategy** — Compare homogeneous clusters vs mixed instance pools
- **Spot-aware** — Factor in spot pricing with configurable spot ratios
- **Scoring** — Weighted scoring across cost, utilization, fragmentation, and resilience (including trough-utilization penalty for over-provisioned night-time clusters)
- **Architecture alternatives** — Auto-compares Intel, AMD, and Graviton families when auto-classification is active
- **What-if analysis** — Compare instance families side by side, with optional workload scaling
- **Offline mode** — Export cluster state as JSON, run simulations without live Prometheus access
- **DaemonSet aware** — Automatically accounts for per-node overhead from DaemonSets
- **Caching** — File-based cache in `~/.cache/clusterfit/` avoids redundant AWS API calls

## Quick Start

### Prerequisites

- **Go 1.25+** to build
- **AWS credentials** with `ec2:DescribeInstanceTypes` permission (the only IAM permission needed — pricing uses a public API, no `pricing:GetProducts` required)
- **Prometheus-compatible endpoint** with standard metrics:
  - `container_cpu_usage_seconds_total` and `container_memory_working_set_bytes` (cAdvisor)
  - `kube_pod_container_resource_requests`, `kube_pod_owner`, `kube_pod_status_phase` (kube-state-metrics)
  - `kube_node_info` (optional, for observed node count range)

#### AWS credential setup

Any standard AWS credential method works:

```bash
# Option 1: SSO (recommended for organizations)
aws sso login --profile my-profile
export AWS_PROFILE=my-profile

# Option 2: Static credentials
aws configure

# Option 3: Environment variables
export AWS_ACCESS_KEY_ID=...
export AWS_SECRET_ACCESS_KEY=...
export AWS_REGION=eu-west-3
```

Minimal IAM policy:

```json
{
  "Version": "2012-10-17",
  "Statement": [{
    "Effect": "Allow",
    "Action": "ec2:DescribeInstanceTypes",
    "Resource": "*"
  }]
}
```

> **Note:** EC2 instance metadata (IMDS) is disabled to avoid long timeouts when running from a laptop. On EC2 instances, use environment variables or an instance profile via `AWS_PROFILE`.

### Install

```bash
go install github.com/guimove/clusterfit@latest
```

Or build from source:

```bash
git clone https://github.com/guimove/clusterfit.git
cd clusterfit
make build
# Binary: ./bin/clusterfit
```

### Usage

**Auto-discover Prometheus and recommend instance types:**

```bash
clusterfit recommend --discover --region eu-west-3
```

**With an explicit Prometheus URL:**

```bash
clusterfit recommend --prometheus-url http://prometheus.monitoring.svc:9090
```

**Inspect cluster workloads (debug metric collection):**

```bash
clusterfit inspect --discover --output json > cluster-state.json
```

**Run offline simulation on a saved snapshot:**

```bash
clusterfit simulate --input cluster-state.json --strategy mixed --top 10
```

**Compare instance families (what-if):**

```bash
clusterfit what-if \
  --input cluster-state.json \
  --baseline m5.xlarge \
  --candidates m6i.xlarge,m7g.xlarge,c6i.xlarge \
  --scale-factor 2.0
```

**List EC2 pricing:**

```bash
clusterfit pricing --families m5,m6i,c6i --region us-west-2 --include-spot
```

### Example output

```
ClusterFit Recommendations
============================================================
Cluster:     prod-eks
Region:      eu-west-3
Pods:        62 (+ 12 DaemonSets)
Percentile:  p95
Window:      2025-01-06 to 2025-01-13
Cluster P95: 12.3 vCPU, 45.6 GiB
Node range:  3 → 8 (observed over window)
Min nodes:   3 (HA constraint)
============================================================

Rank Configuration                     Nodes   CPU%    Mem%  Score   $/month Notes
----------------------------------------------------------------------------------------------------
#1   m7i.xlarge                            5  72.3%  68.1%   81.2      701
#2   m6i.xlarge                            5  72.3%  68.1%   79.8      723
#3   m7i.2xlarge                           3  64.1%  57.2%   74.5      842
#4   m6i.2xlarge                           3  64.1%  57.2%   73.1      867
#5   m7i.large                            10  71.8%  67.5%   68.3     1402 [trough: 18%]
----------------------------------------------------------------------------------------------------
```

## Commands

| Command | Description |
|---------|-------------|
| `recommend` | Full pipeline: collect metrics, fetch instance types, simulate, rank |
| `inspect` | Collect and display current workload state |
| `simulate` | Run simulation on a pre-collected cluster snapshot (JSON) |
| `what-if` | Compare instance configurations side by side |
| `pricing` | List EC2 instance pricing and specs |
| `version` | Print version information |

## How It Works

```
Prometheus ──→ Collect ──→ Size ──→ Fetch ──→ Simulate ──→ Score ──→ Report
  (metrics)   (PromQL)   (pods)   (EC2 API)   (BFD)      (rank)   (table/json/md)
```

1. **Collect** — Queries Prometheus for per-pod CPU/memory percentiles (p50, p95, p99), resource requests/limits, pod ownership (to identify DaemonSets), and cluster-wide aggregate metrics (P95 CPU/memory, min/max node counts over the window)
2. **Size** — Computes effective resource needs per pod: `max(request, observed_usage_at_percentile)`. Floors at 10m CPU / 64 MiB memory to prevent zero-sized pods
3. **Classify** — When no instance families are specified, auto-classifies workloads by GiB/vCPU ratio: compute-optimized (C-series, <3), general-purpose (M-series, 3–6), or memory-optimized (R-series, >6)
4. **Fetch** — Retrieves EC2 instance types via `DescribeInstanceTypes` and enriches with on-demand/spot pricing from a public API (no AWS Pricing permission needed). Results are cached locally
5. **Simulate** — Runs Best Fit Decreasing bin-packing for each candidate instance type. Accounts for system-reserved resources, DaemonSet per-node overhead, and enforces the minimum node count (HA constraint). Computes scaling efficiency based on observed node range
6. **Score** — Ranks candidates by weighted composite score (see Scoring below)
7. **Report** — Outputs top-N recommendations as a table, JSON, or Markdown, with architecture alternatives when auto-classification was used

### Scoring

Each candidate is scored on four dimensions (weights configurable):

| Dimension | Default Weight | What it measures |
|-----------|---------------|-----------------|
| **Cost** | 40% | Cheaper is better (normalized across all candidates) |
| **Utilization** | 30% | Average CPU + memory utilization (higher = less waste) |
| **Fragmentation** | 15% | Resource balance across nodes (penalizes stranded CPU or memory) |
| **Resilience** | 15% | Node count sweet spot (3–15 ideal), DaemonSet overhead penalty at high node counts, unschedulable pod penalty, trough utilization penalty |

**Trough utilization penalty:** When scaling data is available (observed min/max node counts), ClusterFit estimates how well each instance type performs at the cluster's *minimum* scale. Instance types that would leave nodes at <30% CPU utilization during off-peak hours receive a resilience penalty of up to 25 points, discouraging over-provisioning at night.

### Scaling awareness

ClusterFit queries cluster-level aggregate P95 CPU and memory over the full metrics window (e.g. 7 days), plus the observed min and max node count. This captures HPA/autoscaler scaling peaks that a point-in-time pod snapshot misses.

The **scaling ratio** (min_nodes / max_nodes) is used to estimate trough-time resource utilization for each candidate instance type. For example, if the cluster scales between 3 and 10 nodes, a candidate that needs 8 nodes at peak will have ~3 nodes at trough, and ClusterFit checks whether those 3 nodes would be well-utilized or mostly idle.

The **minimum node count** (default: 3) is an HA constraint: even if only 1 node is needed for the workloads, the simulation pads to at least 3 nodes with the cheapest available template.

## Auto-Discovery

ClusterFit can auto-discover your Prometheus-compatible metrics endpoint by searching for well-known Kubernetes service labels. Use `--discover` (or `-d`) to enable it.

**Discovery order** (first match wins):

1. **Thanos** — `app.kubernetes.io/name=thanos` query component
2. **Victoria Metrics** — `app.kubernetes.io/name=vmsingle` or `vmselect`
3. **Grafana Mimir** — `app.kubernetes.io/name=mimir` query-frontend
4. **Cortex** — `app.kubernetes.io/name=cortex` query-frontend
5. **Prometheus** — `kube-prometheus-stack`, `prometheus-server`, etc.

When running outside the cluster, ClusterFit automatically sets up a port-forward tunnel to the discovered service — no manual `kubectl port-forward` needed.

```bash
# Search all namespaces
clusterfit recommend --discover

# Limit to a specific namespace
clusterfit recommend --discover --discovery-namespace monitoring

# Use a specific kubeconfig / context
clusterfit recommend --discover --kubeconfig ~/.kube/prod --kube-context prod-cluster
```

## Configuration

Configuration can be set via (highest priority wins):

1. CLI flags
2. Environment variables (`CLUSTERFIT_PROMETHEUS_URL`, etc.)
3. Config file (`clusterfit.yaml` or `--config path`)
4. Built-in defaults

Copy the example config and customize:

```bash
cp clusterfit.yaml.example clusterfit.yaml
```

See [`clusterfit.yaml.example`](clusterfit.yaml.example) for all available options.

### CLI Flags

#### Global flags (all commands)

| Flag | Description |
|------|-------------|
| `--config` | Path to config file (default: `clusterfit.yaml` in `.` or `~/.clusterfit/`) |
| `--region` | AWS region (default: `AWS_REGION` env var, then `us-east-1`) |
| `--prometheus-url` | Prometheus/Thanos endpoint URL |
| `--discover` / `-d` | Auto-discover metrics endpoint from Kubernetes |
| `--discovery-namespace` | Limit auto-discovery to a namespace |
| `--kubeconfig` | Path to kubeconfig file |
| `--kube-context` | Kubernetes context name |
| `--verbose` | Enable verbose output |

#### `recommend` flags

| Flag | Config Key | Default | Description |
|------|-----------|---------|-------------|
| `--window` | `metrics.window` | `168h` | Metrics lookback window |
| `--percentile` | `metrics.percentile` | `0.95` | Sizing percentile (0.0–1.0) |
| `--families` | `instances.families` | auto | EC2 families to evaluate |
| `--architectures` | `instances.architectures` | `amd64` | CPU architectures |
| `--spot-ratio` | `simulation.spot_ratio` | `0.0` | Spot fraction (0.0–1.0) |
| `--exclude-namespaces` | `metrics.exclude_namespaces` | kube-system,... | Namespaces to exclude |
| `--top` | `output.top_n` | `5` | Number of recommendations |
| `--output` | `output.format` | `table` | Output format: table, json, markdown |
| `--output-file` | — | stdout | Write output to file |
| `--no-cache` | — | false | Disable file-based caching |

#### `simulate` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(required)* | Path to cluster state JSON (from `inspect --output json`) |
| `--strategy` | `both` | Simulation strategy: homogeneous, mixed, or both |
| `--spot-ratio` | `0.0` | Spot fraction |
| `--output` | `table` | Output format |
| `--top` | `5` | Number of recommendations |

#### `what-if` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--input` | *(required)* | Path to cluster state JSON |
| `--baseline` | — | Baseline instance type (e.g. `m5.xlarge`) |
| `--candidates` | — | Comma-separated instance types to compare |
| `--scale-factor` | `1.0` | Multiply workload count (for growth planning) |
| `--output` | `table` | Output format |

#### `pricing` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--families` | from config | Instance families to list |
| `--architectures` | `amd64` | Filter by architecture |
| `--sort-by` | `price` | Sort by: price, vcpu, memory, type |
| `--include-spot` | false | Show spot prices alongside on-demand |

#### `inspect` flags

| Flag | Default | Description |
|------|---------|-------------|
| `--window` | `168h` | Metrics lookback window |
| `--percentile` | `0.95` | Sizing percentile |
| `--output` | `table` | Output format: table, json |
| `--sort-by` | `cpu` | Sort workloads by: cpu, memory, name |
| `--output-file` | stdout | Write output to file |

### Config-only options

These options are only available in the config file, not as CLI flags:

| Config Key | Default | Description |
|-----------|---------|-------------|
| `simulation.min_nodes` | `3` | Minimum node count (HA constraint). Set to 0 to disable |
| `simulation.max_nodes` | `500` | Maximum node count per scenario |
| `simulation.system_reserved.cpu_millis` | `100` | CPU reserved for kubelet/system per node |
| `simulation.system_reserved.memory_mib` | `256` | Memory reserved for kubelet/system per node |
| `instances.exclude_burstable` | `true` | Exclude T-family instances |
| `instances.exclude_bare_metal` | `true` | Exclude bare-metal instances |
| `instances.current_generation_only` | `true` | Only current-generation instances |
| `instances.min_vcpus` | `2` | Minimum vCPUs per instance |
| `instances.max_vcpus` | `96` | Maximum vCPUs per instance |
| `scoring.weights.*` | see below | Scoring dimension weights (must sum to 1.0) |

## Offline workflow

ClusterFit supports a collect-once, simulate-many workflow:

```bash
# 1. Collect metrics from the cluster (requires Prometheus access)
clusterfit inspect --discover --output json > cluster-state.json

# 2. Run simulations offline (no Prometheus needed, only AWS credentials)
clusterfit simulate --input cluster-state.json --strategy both --top 10

# 3. Compare specific instance types
clusterfit what-if \
  --input cluster-state.json \
  --baseline m5.xlarge \
  --candidates m7i.xlarge,m7g.xlarge,c7i.xlarge

# 4. Re-simulate with different parameters
clusterfit simulate --input cluster-state.json --spot-ratio 0.7 --output markdown
```

The `simulate` command uses a built-in set of common instance types (m5, m6i, m7g, c5, r5 families) for offline simulation without needing AWS API access.

## Development

```bash
make build         # Build binary to bin/clusterfit
make test          # Run all tests with race detection
make bench         # Run BFD benchmarks
make lint          # Run golangci-lint
make vet           # Run go vet
make fmt           # Format code
make tidy          # go mod tidy
make all           # tidy + fmt + vet + test + build
```

### Project structure

```
cmd/                          CLI commands (Cobra)
  recommend.go                Full pipeline command
  inspect.go                  Workload inspection
  simulate.go                 Offline simulation
  whatif.go                   Instance type comparison
  pricing.go                  EC2 pricing lookup
  discovery.go                Metrics endpoint auto-discovery
internal/
  model/                      Core types (zero dependencies)
    cluster.go                ClusterState, ClusterAggregateMetrics, workload classification
    result.go                 SimulationResult, ScalingEfficiency, Recommendation
    workload.go               WorkloadProfile, ResourceQuantity, PercentileValues
    node.go                   NodeTemplate, Architecture, CapacityType
  simulation/                 Bin-packing engine
    bfd.go                    Best Fit Decreasing algorithm (MinNodes enforcement)
    engine.go                 Parallel scenario runner, ScalingEfficiency computation
    scorer.go                 Composite scoring with trough-utilization penalty
    fragmentation.go          Stranded resource and balance analysis
    packer.go                 BinPacker interface, PackInput/PackResult
  metrics/                    Metrics collection
    prometheus.go             Prometheus/Thanos/Cortex collector
    queries.go                PromQL templates (per-pod + cluster aggregate)
    static.go                 Static collector (from JSON files)
    collector.go              MetricsCollector interface
  aws/                        AWS integration
    provider.go               AWSProvider (ec2:DescribeInstanceTypes)
    instances.go              Instance type fetching and filtering
    pricing.go                Public pricing API (runs-on.com, no auth)
    cache.go                  File-based cache (~/.cache/clusterfit/)
  report/                     Output formatters
    table.go                  Terminal table output
    markdown.go               Markdown output
    reporter.go               Reporter interface, JSON reporter
  config/                     Configuration types and defaults
  orchestrator/               End-to-end pipeline coordinator
  kube/                       Kubernetes client, service discovery, port-forwarding
testdata/
  metrics/small_cluster.json  5 workloads + 2 DaemonSets
  metrics/medium_cluster.json 100 workloads + 4 DaemonSets
```

## License

[Apache License 2.0](LICENSE)
