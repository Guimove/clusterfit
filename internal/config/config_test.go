package config

import (
	"testing"
)

func TestDefault_Valid(t *testing.T) {
	cfg := Default()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("default config should be valid: %v", err)
	}
}

func TestValidate_InvalidPercentile(t *testing.T) {
	cfg := Default()
	cfg.Metrics.Percentile = 1.5
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for percentile > 1.0")
	}

	cfg.Metrics.Percentile = -0.1
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for negative percentile")
	}
}

func TestValidate_InvalidWindow(t *testing.T) {
	cfg := Default()
	cfg.Metrics.Window = 0
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for zero window")
	}
}

func TestValidate_InvalidSpotRatio(t *testing.T) {
	cfg := Default()
	cfg.Simulation.SpotRatio = 1.5
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for spot ratio > 1.0")
	}
}

func TestValidate_InvalidStrategy(t *testing.T) {
	cfg := Default()
	cfg.Simulation.Strategy = "invalid"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid strategy")
	}
}

func TestValidate_InvalidFormat(t *testing.T) {
	cfg := Default()
	cfg.Output.Format = "xml"
	if err := cfg.Validate(); err == nil {
		t.Error("expected error for invalid output format")
	}
}

func TestValidate_TopN_FixesZero(t *testing.T) {
	cfg := Default()
	cfg.Output.TopN = 0
	if err := cfg.Validate(); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Output.TopN != 5 {
		t.Errorf("expected TopN to be fixed to 5, got %d", cfg.Output.TopN)
	}
}
