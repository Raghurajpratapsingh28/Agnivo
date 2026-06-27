// Package app wires the proxy-manager executable onto the shared platform
// bootstrap, initialising the complete edge networking module.
package app

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/proxy"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
)

// Register attaches the edge networking module to the application.
func Register(ctx context.Context, a *bootstrap.App) error {
	_, err := proxy.Init(ctx, a)
	return err
}
