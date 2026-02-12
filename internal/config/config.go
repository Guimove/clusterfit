package config

import (
	"fmt"
	"os"
	"time"
)

// Config is the top-level configuration for ClusterFit.
type Config struct {
	Cluster    ClusterConfig    `yaml:"cluster"`
	Prometheus PrometheusConfig `yaml:"prometheus"`
	Kubernetes KubernetesConfig `yaml:"kubernetes"`
	Metrics    MetricsConfig    `yaml:"metrics"`
	Instances  InstancesConfig  `yaml:"instances"`
	Simulation SimulationConfig `yaml:"simulation"`
	Scoring    ScoringConfig    `yaml:"scoring"`
	Output     OutputConfig     `yaml:"output"`
}

type KubernetesConfig struct {
	Enabled            bool   `yaml:"enabled"`
	Kubeconfig         string `yaml:"kubeconfig"`
	Context            string `yaml:"context"`
	DiscoveryNamespace string `yaml:"discovery_namespace"` // empty = all namespaces
}

type ClusterConfig struct {
	Name   string `yaml:"name"`
	Region string `yaml:"region"`
}

type PrometheusConfig struct {
	URL     string        `yaml:"url"`
	Timeout time.Duration `yaml:"timeout"`
}

type MetricsConfig struct {
	Window            time.Duration `yaml:"window"`
	Step              time.Duration `yaml:"step"`
	Percentile        float64       `yaml:"percentile"`
	ExcludeNamespaces []string      `yaml:"exclude_namespaces"`
}

type InstancesConfig struct {
	Families              []string `yaml:"families"`
	Architectures         []string `yaml:"architectures"`
	ExcludeBurstable      bool     `yaml:"exclude_burstable"`
	ExcludeBareMetal      bool     `yaml:"exclude_bare_metal"`
	CurrentGenerationOnly bool     `yaml:"current_generation_only"`
	MinVCPUs              int32    `yaml:"min_vcpus"`
	MaxVCPUs              int32    `yaml:"max_vcpus"`
}

type SimulationConfig struct {
	Strategy       string             `yaml:"strategy"`
	SpotRatio      float64            `yaml:"spot_ratio"`
	SystemReserved SystemReservedConf `yaml:"system_reserved"`
	MaxNodes       int                `yaml:"max_nodes"`
	MinNodes       int                `yaml:"min_nodes"`
}

type SystemReservedConf struct {
	CPUMillis int64 `yaml:"cpu_millis"`
	MemoryMiB int64 `yaml:"memory_mib"`
}

type ScoringConfig struct {
	Weights ScoringWeightsConf `yaml:"weights"`
}

type ScoringWeightsConf struct {
	Cost          float64 `yaml:"cost"`
	Utilization   float64 `yaml:"utilization"`
	Fragmentation float64 `yaml:"fragmentation"`
	Resilience    float64 `yaml:"resilience"`
}

type OutputConfig struct {
	Format string `yaml:"format"`
	TopN   int    `yaml:"top_n"`
}

// Default returns a Config with sensible defaults.
func Default() Config {
	return Config{
		Cluster: ClusterConfig{
			Region: detectRegion(),
		},
		Prometheus: PrometheusConfig{
			Timeout: 60 * time.Second,
		},
		Metrics: MetricsConfig{
			Window:     7 * 24 * time.Hour,
			Step:       5 * time.Minute,
			Percentile: 0.95,
			ExcludeNamespaces: []string{
				"kube-system",
				"kube-node-lease",
				"karpenter",
			},
		},
		Instances: InstancesConfig{
			Families:              nil, // auto-selected from workload classification when empty
			Architectures:         []string{"amd64"},
			ExcludeBurstable:      true,
			ExcludeBareMetal:      true,
			CurrentGenerationOnly: true,
			MinVCPUs:              2,
			MaxVCPUs:              96,
		},
		Simulation: SimulationConfig{
			Strategy:  "both",
			SpotRatio: 0.0,
			SystemReserved: SystemReservedConf{
				CPUMillis: 100,
				MemoryMiB: 256,
			},
			MaxNodes: 500,
			MinNodes: 3,
		},
		Scoring: ScoringConfig{
			Weights: ScoringWeightsConf{
				Cost:          0.40,
				Utilization:   0.30,
				Fragmentation: 0.15,
				Resilience:    0.15,
			},
		},
		Output: OutputConfig{
			Format: "table",
			TopN:   5,
		},
	}
}

// Validate checks the config for consistency.
func (c *Config) Validate() error {
	if c.Metrics.Percentile < 0 || c.Metrics.Percentile > 1.0 {
		return fmt.Errorf("percentile must be between 0 and 1.0, got %v", c.Metrics.Percentile)
	}
	if c.Metrics.Window <= 0 {
		return fmt.Errorf("metrics window must be positive, got %v", c.Metrics.Window)
	}
	if c.Simulation.SpotRatio < 0 || c.Simulation.SpotRatio > 1.0 {
		return fmt.Errorf("spot_ratio must be between 0 and 1.0, got %v", c.Simulation.SpotRatio)
	}
	if c.Simulation.MinNodes < 0 {
		return fmt.Errorf("min_nodes must be non-negative, got %d", c.Simulation.MinNodes)
	}
	validStrats := map[string]bool{"homogeneous": true, "mixed": true, "both": true}
	if !validStrats[c.Simulation.Strategy] {
		return fmt.Errorf("strategy must be homogeneous, mixed, or both, got %q", c.Simulation.Strategy)
	}
	validFormats := map[string]bool{"table": true, "json": true, "markdown": true, "csv": true}
	if !validFormats[c.Output.Format] {
		return fmt.Errorf("output format must be table, json, markdown, or csv, got %q", c.Output.Format)
	}
	if c.Output.TopN <= 0 {
		c.Output.TopN = 5
	}
	return nil
}

// detectRegion checks environment variables for the AWS region.
func detectRegion() string {
	if r := os.Getenv("AWS_REGION"); r != "" {
		return r
	}
	if r := os.Getenv("AWS_DEFAULT_REGION"); r != "" {
		return r
	}
	return "us-east-1"
}
