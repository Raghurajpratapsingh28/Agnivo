// Package app wires the api executable's routes and runners onto the shared
// platform bootstrap. It contains no infrastructure setup — that is owned by
// packages/platform/bootstrap.
package app

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes"
	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/streaming"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
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
