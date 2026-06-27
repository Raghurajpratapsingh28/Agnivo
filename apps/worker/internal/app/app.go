// Package app wires the worker executable onto the shared platform bootstrap,
// registering all Platform Operations background job handlers.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/ops"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the ops module (worker mode) to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := ops.Init(ctx, a, true, false)
	return err
}
