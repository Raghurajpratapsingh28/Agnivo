// Package app wires the builder executable onto the shared platform bootstrap.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/build"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the builder job worker and internal API to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := build.Init(ctx, a)
	return err
}
