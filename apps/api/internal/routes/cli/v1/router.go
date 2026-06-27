// Package v1 mounts CLI/CI-optimized routes under /cli/v1.
package v1

import (
	"github.com/agnivo/agnivo/packages/platform/httpx"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// NewRouter builds the /cli/v1 routes.
func NewRouter(log *zap.Logger) chi.Router {
	r := chi.NewRouter()

	r.Post("/auth/login", httpx.NotImplemented)
	r.Post("/deploy", httpx.NotImplemented)
	r.Post("/rollback", httpx.NotImplemented)
	r.Get("/logs", httpx.NotImplemented)

	r.Route("/domains", func(r chi.Router) {
		r.Get("/", httpx.NotImplemented)
		r.Post("/", httpx.NotImplemented)
	})
	r.Route("/secrets", func(r chi.Router) {
		r.Get("/", httpx.NotImplemented)
		r.Post("/", httpx.NotImplemented)
	})
	r.Route("/env", func(r chi.Router) {
		r.Get("/", httpx.NotImplemented)
		r.Put("/", httpx.NotImplemented)
	})
	r.Route("/tokens", func(r chi.Router) {
		r.Get("/", httpx.NotImplemented)
		r.Post("/", httpx.NotImplemented)
		r.Delete("/{tokenID}", httpx.NotImplemented)
	})

	_ = log
	return r
}
