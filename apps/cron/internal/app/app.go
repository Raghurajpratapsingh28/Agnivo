// Package app wires the cron executable onto the shared platform bootstrap,
// registering the distributed cron scheduler and seeding built-in schedules.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/ops"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the ops module (cron mode) to the application.
// The cron executable only runs the scheduler — workers are separate processes.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := ops.Init(ctx, a, false, true)
	return err
}
