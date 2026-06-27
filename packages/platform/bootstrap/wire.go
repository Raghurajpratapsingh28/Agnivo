//go:build wireinject
// +build wireinject

package bootstrap

import (
	"context"

	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/google/wire"
)

// initCore is the Wire injector that assembles Core from configuration. The
// concrete implementation is generated into wire_gen.go.
func initCore(ctx context.Context, cfg *config.Config) (*Core, error) {
	wire.Build(ProviderSet)
	return nil, nil
}
