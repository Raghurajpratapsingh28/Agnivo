// Package webhooks mounts git provider webhook ingestion under /webhooks.
package webhooks

import (
	"net/http"

	"github.com/agnivo/agnivo/packages/application/controlplane"
	"github.com/agnivo/agnivo/packages/platform/dto"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// NewRouter builds the webhook routes with signature verification and deployment triggers.
func NewRouter(log *zap.Logger, cp *controlplane.Module) chi.Router {
	r := chi.NewRouter()
	if cp == nil {
		return r
	}
	for _, provider := range []string{"github", "gitlab", "bitbucket"} {
		p := provider
		r.Post("/"+p, func(w http.ResponseWriter, req *http.Request) {
			if err := cp.Webhooks.Handle(req.Context(), p, req); err != nil {
				log.Debug("webhook rejected", zap.String("provider", p), zap.Error(err))
				dto.Error(w, req, err)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		})
	}
	return r
}
