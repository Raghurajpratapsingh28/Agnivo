// Package v1 mounts live streaming routes under /stream/v1.
package v1

import (
	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/streaming"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// NewRouter builds the /stream/v1 routes backed by the streaming hub.
func NewRouter(log *zap.Logger, hub *streaming.Hub) chi.Router {
	r := chi.NewRouter()

	r.Get("/logs", httpx.NotImplemented)
	r.Get("/builds/{buildID}", httpx.NotImplemented)
	r.Get("/deployments/{deploymentID}", httpx.NotImplemented)
	r.Get("/metrics", httpx.NotImplemented)
	r.Get("/notifications/ws", httpx.NotImplemented)

	_ = log
	_ = hub
	return r
}
