// Package app wires the scheduler executable onto the shared platform bootstrap.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/scheduler"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the scheduler service to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := scheduler.Init(ctx, a)
	return err
}
