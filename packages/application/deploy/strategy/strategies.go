package strategy

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/deploy/runtime"
)

// Rolling performs a standard rolling deployment (create new, drain old).
type Rolling struct{}

// Deploy creates and starts a new container.
func (r *Rolling) Deploy(ctx context.Context, sctx Context, drv runtime.Driver) (Result, error) {
	cfg := sctx.Runtime
	cfg.HostPort = sctx.Placement.Port
	cfg.Network = ""
	container, err := drv.Create(ctx, sctx.DeploymentID, cfg)
	if err != nil {
		return Result{}, err
	}
	if err := drv.Start(ctx, container.ID); err != nil {
		_ = drv.Remove(ctx, container.ID)
		return Result{}, err
	}
	container.Host = sctx.Placement.Host
	container.Port = sctx.Placement.Port
	return Result{Container: container}, nil
}

// BlueGreen deploys new container alongside old, then switches traffic.
type BlueGreen struct{ base *Rolling }

func (b *BlueGreen) Deploy(ctx context.Context, sctx Context, drv runtime.Driver) (Result, error) {
	res, err := b.base.Deploy(ctx, sctx, drv)
	if err != nil {
		return res, err
	}
	// Traffic switch handled by executor after health checks; mark role as green
	return res, nil
}

// Canary deploys canary instance (same as rolling with preview routing metadata).
type Canary struct{ base *Rolling }

func (c *Canary) Deploy(ctx context.Context, sctx Context, drv runtime.Driver) (Result, error) {
	sctx.Runtime.Labels["agnivo.canary"] = "true"
	return c.base.Deploy(ctx, sctx, drv)
}

// Preview deploys to isolated preview environment.
type Preview struct{ base *Rolling }

func (p *Preview) Deploy(ctx context.Context, sctx Context, drv runtime.Driver) (Result, error) {
	sctx.Runtime.Labels["agnivo.preview"] = "true"
	return p.base.Deploy(ctx, sctx, drv)
}

// Immediate deploys without waiting for drain (rollback/fast path).
type Immediate struct{ base *Rolling }

func (i *Immediate) Deploy(ctx context.Context, sctx Context, drv runtime.Driver) (Result, error) {
	return i.base.Deploy(ctx, sctx, drv)
}
