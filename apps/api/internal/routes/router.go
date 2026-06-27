// Package routes assembles the api executable's HTTP surfaces onto the shared
// router. Health and metrics are provided by the platform bootstrap.
package routes

import (
	"net/http"

	apimw "github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/middleware"
	cliv1 "github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes/cli/v1"
	internalv1 "github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes/internal/v1"
	realtimev1 "github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes/realtime/v1"
	v1 "github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes/v1"
	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/routes/webhooks"
	"github.com/Raghurajpratapsingh28/Agnivo/apps/api/internal/streaming"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Mount attaches all api surfaces to the router:
//   - /api/v1       dashboard control plane
//   - /cli/v1       CLI and CI/CD
//   - /stream/v1    SSE and WebSocket live updates
//   - /internal/v1  scheduler placement (service-to-service)
//   - /webhooks     git provider webhooks
func Mount(r chi.Router, log *zap.Logger, cfg *config.Config, hub *streaming.Hub, id *identity.Module, cp *controlplane.Module) {
	r.Get("/", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"service":"api","status":"ok"}`))
	})

	r.Mount("/webhooks", webhooks.NewRouter(log, cp))
	r.Mount("/api/v1", v1.NewRouter(log, id, cp))
	r.Mount("/cli/v1", cliv1.NewRouter(log))

	r.Group(func(r chi.Router) {
		if id != nil {
			r.Use(apimw.StreamAuth(id.Middleware))
		}
		r.Mount("/stream/v1", realtimev1.NewRouter(log, hub))
	})

	r.Group(func(r chi.Router) {
		r.Use(apimw.InternalAuth(cfg))
		r.Mount("/internal/v1", internalv1.NewRouter(log))
	})
}
