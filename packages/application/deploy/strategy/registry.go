package strategy

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/runtime"
)

// Context carries deployment execution state for strategies.
type Context struct {
	DeploymentID string
	Environment  string
	Strategy     string
	Runtime      model.RuntimeConfig
	Placement    Placement
	IsPreview    bool
	IsRollback   bool
}

// Placement holds scheduler placement info.
type Placement struct {
	Host string
	Port int
}

// Result is the outcome of a strategy execution.
type Result struct {
	Container runtime.ContainerInfo
	Drained   []string
}

// Executor runs a deployment strategy.
type Executor interface {
	Deploy(ctx context.Context, sctx Context, runtime runtime.Driver) (Result, error)
}

// Registry maps strategy names to executors.
type Registry struct {
	executors map[string]Executor
	defaultEx Executor
}

// NewRegistry constructs a strategy registry with built-in strategies.
func NewRegistry(defaultStrategy string) *Registry {
	rolling := &Rolling{}
	r := &Registry{
		executors: map[string]Executor{
			model.StrategyRolling:   rolling,
			model.StrategyBlueGreen: &BlueGreen{base: rolling},
			model.StrategyCanary:    &Canary{base: rolling},
			model.StrategyPreview:   &Preview{base: rolling},
			model.StrategyImmediate: &Immediate{base: rolling},
		},
		defaultEx: rolling,
	}
	if ex, ok := r.executors[defaultStrategy]; ok {
		r.defaultEx = ex
	}
	return r
}

// Get returns the executor for a strategy name.
func (r *Registry) Get(name string) Executor {
	if ex, ok := r.executors[name]; ok {
		return ex
	}
	return r.defaultEx
}

// Resolve picks strategy based on environment and config default.
func Resolve(environment, defaultStrategy string, isPreview, isRollback bool) string {
	if isRollback {
		return model.StrategyImmediate
	}
	if isPreview || environment == "preview" {
		return model.StrategyPreview
	}
	if defaultStrategy != "" {
		return defaultStrategy
	}
	return model.StrategyRolling
}
