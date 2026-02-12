# ClusterFit

EC2 instance sizing recommender for EKS clusters. ClusterFit analyzes real pod resource usage from Prometheus, runs bin-packing simulations across instance types, and ranks them by cost, utilization, and fragmentation.

Think of it as [Goldilocks](https://github.com/FairwindsOps/goldilocks) for **nodes** instead of pods — finding instance types that are *just right* for your workload mix.

## Features

- **Metrics-driven** — Collects actual CPU/memory usage from Prometheus, Thanos, Cortex, Victoria Metrics, or Mimir
- **Auto-discovery** — Finds your metrics endpoint in Kubernetes automatically (KRR-style `--discover` flag)
- **Bin-packing simulation** — Best Fit Decreasing algorithm packs your workloads into candidate instance types
- **Multi-strategy** — Compare homogeneous clusters vs mixed instance pools
- **Spot-aware** — Factor in spot pricing with configurable spot ratios
- **Scoring** — Weighted scoring across cost, utilization, fragmentation, and resilience
- **What-if analysis** — Compare instance families side by side, with optional workload scaling
- **Offline mode** — Export cluster state as JSON, run simulations without live Prometheus access
- **DaemonSet aware** — Automatically accounts for per-node overhead from DaemonSets

## Quick Start

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
clusterfit recommend --discover
```

**With an explicit Prometheus URL:**

```bash
clusterfit recommend --prometheus-url http://prometheus.monitoring.svc:9090
```

**Inspect cluster workloads:**

```bash
clusterfit inspect --discover --output json > cluster-state.json
```

**Run offline simulation:**

```bash
clusterfit simulate --input cluster-state.json --strategy mixed --top 10
```

**Compare instance families (what-if):**

```bash
clusterfit what-if \
  --input cluster-state.json \
  --baseline m5.xlarge \
  --candidates m6i.xlarge,m7g.xlarge,c6i.xlarge
```

**List EC2 pricing:**

```bash
clusterfit pricing --families m5,m6i,c6i --region us-west-2
```

## Commands

| Command | Description |
|---------|-------------|
| `recommend` | Full pipeline: collect metrics, fetch pricing, simulate, rank |
| `inspect` | Collect and display current workload state |
| `simulate` | Run simulation on a pre-collected cluster snapshot (JSON) |
| `what-if` | Compare instance configurations side by side |
| `pricing` | List EC2 instance pricing and specs |
| `version` | Print version information |

## Auto-Discovery

ClusterFit can auto-discover your Prometheus-compatible metrics endpoint by searching for well-known Kubernetes service labels. Use `--discover` (or `-d`) to enable it.

**Discovery order** (first match wins):

1. **Thanos** — `app.kubernetes.io/name=thanos` query component
2. **Victoria Metrics** — `app.kubernetes.io/name=vmsingle` or `vmselect`
3. **Grafana Mimir** — `app.kubernetes.io/name=mimir` query-frontend
4. **Cortex** — `app.kubernetes.io/name=cortex` query-frontend
5. **Prometheus** — `kube-prometheus-stack`, `prometheus-server`, etc.

```bash
# Search all namespaces
clusterfit recommend --discover

# Limit to a specific namespace
clusterfit recommend --discover --discovery-namespace monitoring

# Use a specific kubeconfig / context
clusterfit recommend --discover --kubeconfig ~/.kube/prod --kube-context prod-cluster
```

## Configuration

Copy the example config and customize:

```bash
cp clusterfit.yaml.example clusterfit.yaml
```

See [`clusterfit.yaml.example`](clusterfit.yaml.example) for all available options. Configuration can be set via:

1. Config file (`clusterfit.yaml` or `--config path`)
2. Environment variables (`CLUSTERFIT_PROMETHEUS_URL`, etc.)
3. CLI flags (highest priority)

### Key Options

| Flag | Config Key | Description |
|------|-----------|-------------|
| `--prometheus-url` | `prometheus.url` | Metrics endpoint URL |
| `--discover` / `-d` | `kubernetes.enabled` | Auto-discover endpoint |
| `--region` | `cluster.region` | AWS region (default: from env) |
| `--window` | `metrics.window` | Metrics lookback (default: `168h`) |
| `--percentile` | `metrics.percentile` | Sizing percentile (default: `0.95`) |
| `--families` | `instances.families` | EC2 families to evaluate |
| `--spot-ratio` | `simulation.spot_ratio` | Spot fraction (`0.0`-`1.0`) |
| `--output` | `output.format` | `table`, `json`, `markdown`, `csv` |

## How It Works

1. **Collect** — Query Prometheus for CPU/memory usage percentiles (p50, p95, p99), resource requests/limits, and pod ownership (to identify DaemonSets)
2. **Size** — Compute effective resource needs per pod: `max(request, observed usage at configured percentile)`
3. **Fetch** — Retrieve EC2 instance types and On-Demand/Spot pricing from AWS APIs
4. **Simulate** — Run Best Fit Decreasing bin-packing for each candidate instance type, accounting for system reserved resources and DaemonSet per-node overhead
5. **Score** — Rank candidates by weighted criteria: cost efficiency (40%), resource utilization (30%), fragmentation (15%), resilience (15%)
6. **Report** — Output the top-N recommendations as a table, JSON, or Markdown

## Requirements

- **Go 1.21+** to build
- **AWS credentials** configured (`aws configure` or IAM role) for EC2 + Pricing API access
- **Prometheus-compatible endpoint** (direct URL or in-cluster auto-discovery)

## License

[Apache License 2.0](LICENSE)
