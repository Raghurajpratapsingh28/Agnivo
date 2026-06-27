// Package app wires the scheduler executable onto the shared platform bootstrap.
package app

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
)

// Register attaches the scheduler service to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := scheduler.Init(ctx, a)
	return err
}
