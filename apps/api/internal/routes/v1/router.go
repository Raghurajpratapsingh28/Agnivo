// Package v1 mounts the dashboard control-plane API.
package v1

import (
	"github.com/agnivo/agnivo/packages/application/controlplane"
	"github.com/agnivo/agnivo/packages/application/identity"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// NewRouter builds the /api/v1 control-plane routes.
func NewRouter(log *zap.Logger, id *identity.Module, cp *controlplane.Module) chi.Router {
	r := chi.NewRouter()

	if id != nil {
		id.MountRoutes(r)
	}
	if cp != nil && id != nil {
		cp.MountRoutes(r, id)
	}

	_ = log
	return r
}
