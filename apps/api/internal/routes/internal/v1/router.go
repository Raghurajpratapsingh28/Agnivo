// Package v1 mounts internal service-to-service routes under /internal/v1
// (scheduler placement, used by workers on the same host or via mTLS).
package v1

import (
	"github.com/agnivo/agnivo/packages/platform/httpx"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// NewRouter builds the /internal/v1 routes.
func NewRouter(log *zap.Logger) chi.Router {
	r := chi.NewRouter()

	r.Post("/placement", httpx.NotImplemented)
	r.Post("/reserve", httpx.NotImplemented)
	r.Post("/release", httpx.NotImplemented)
	r.Get("/capacity", httpx.NotImplemented)

	_ = log
	return r
}
