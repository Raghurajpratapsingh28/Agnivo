// Package app wires the runtime-agent executable onto the shared platform bootstrap.
package app

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

// Register attaches the runtime agent service to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := runtimeagent.Init(ctx, a)
	return err
}
