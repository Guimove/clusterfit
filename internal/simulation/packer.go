package simulation

import (
	"context"

	"github.com/guimove/clusterfit/internal/model"
)

// BinPacker defines a strategy for placing workloads onto nodes.
type BinPacker interface {
	// Pack attempts to place all workloads onto nodes of the given templates.
	Pack(ctx context.Context, input PackInput) (*PackResult, error)

	// Name returns the strategy name.
	Name() string
}

// PackInput is the input to a bin-packing run.
type PackInput struct {
	Workloads      []model.WorkloadProfile
	DaemonSets     []model.WorkloadProfile
	NodeTemplates  []model.NodeTemplate
	SystemReserved model.ResourceQuantity
	MaxNodes       int     // 0 = unlimited
	SpotRatio      float64 // Fraction of nodes to be spot (0.0 - 1.0)
}

// PackResult is the output of a bin-packing run.
type PackResult struct {
	Nodes             []model.NodeAllocation
	UnschedulablePods []model.WorkloadProfile
}
