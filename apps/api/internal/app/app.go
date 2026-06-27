// Package app wires the api executable's routes and runners onto the shared
// platform bootstrap. It contains no infrastructure setup — that is owned by
// packages/platform/bootstrap.
package app

import (
	"context"

	"github.com/agnivo/agnivo/apps/api/internal/routes"
	"github.com/agnivo/agnivo/apps/api/internal/streaming"
	"github.com/agnivo/agnivo/packages/application/controlplane"
	"github.com/agnivo/agnivo/packages/application/identity"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
	"github.com/agnivo/agnivo/packages/platform/lifecycle"
)

// Register attaches the api routes and the streaming hub to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	hub := streaming.NewHub(a.Log)

	a.AddRunner("streaming-hub", hub.Run)
	a.AddHook(lifecycle.Hook{Name: "streaming-hub", Stop: hub.Shutdown})

	id, err := identity.Init(ctx, a)
	if err != nil {
		return err
	}

	cp, err := controlplane.Init(ctx, a, id)
	if err != nil {
		return err
	}

	routes.Mount(a.Router, a.Log, a.Config, hub, id, cp)
	return nil
}
