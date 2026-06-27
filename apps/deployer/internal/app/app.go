// Package app wires the deployer executable onto the shared platform bootstrap.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/deploy"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the deployer job worker and internal API to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := deploy.Init(ctx, a)
	return err
}
