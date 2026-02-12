# Contributing to ClusterFit

Thank you for your interest in contributing to ClusterFit! This document covers the process for contributing to this project.

## Getting Started

1. Fork the repository
2. Clone your fork:
   ```bash
   git clone https://github.com/<your-user>/clusterfit.git
   cd clusterfit
   ```
3. Create a branch:
   ```bash
   git checkout -b feat/my-feature
   ```
4. Make your changes and verify:
   ```bash
   make all   # runs: tidy, fmt, vet, test, build
   ```
5. Push and open a pull request

## Development

### Prerequisites

- Go 1.21+
- `golangci-lint` (optional, for `make lint`)

### Build & Test

```bash
make build          # Build binary to bin/clusterfit
make test           # Run all tests with race detection
make bench          # Benchmark the bin-packing engine
make lint           # Run golangci-lint
make vet            # Run go vet
make fmt            # Format code
make tidy           # Run go mod tidy
make all            # All of the above
```

### Project Structure

```
cmd/                    CLI commands (Cobra)
internal/
  aws/                  EC2 instance types & pricing
  config/               Configuration loading & validation
  kube/                 Kubernetes client & service discovery
  metrics/              Prometheus/Thanos metrics collection
  model/                Core types (zero external dependencies)
  orchestrator/         End-to-end pipeline coordination
  report/               Output formatters (table, JSON, markdown)
  simulation/           BFD bin-packing engine, scorer, fragmentation
pkg/version/            Version info (injected at build)
testdata/               Test fixtures
```

### Design Principles

- **`internal/model/` has zero external dependencies** — it only uses the Go standard library. Keep it that way.
- **Prefer table-driven tests** — see `internal/simulation/` for examples.
- **Avoid over-engineering** — a simple solution that works is better than a flexible one that doesn't.
- **Test with fakes, not mocks** — use `fake.NewSimpleClientset()` for Kubernetes, static collectors for metrics.

## Pull Requests

- Keep PRs focused on a single concern
- Include tests for new functionality
- Run `make all` before submitting
- Write a clear PR description explaining *why*, not just *what*

### Commit Messages

Use conventional commits:

```
feat: add arm64 instance support
fix: handle empty metrics window gracefully
docs: update configuration examples
test: add benchmark for mixed-strategy simulation
refactor: extract port selection logic
```

## Reporting Issues

When filing a bug report, please include:

- ClusterFit version (`clusterfit version`)
- Go version (`go version`)
- What you expected vs what happened
- Steps to reproduce
- Relevant config (sanitize credentials)

## Code of Conduct

Be respectful, constructive, and inclusive. We follow the [Contributor Covenant](https://www.contributor-covenant.org/version/2/1/code_of_conduct/).

## License

By contributing, you agree that your contributions will be licensed under the [Apache License 2.0](LICENSE).
