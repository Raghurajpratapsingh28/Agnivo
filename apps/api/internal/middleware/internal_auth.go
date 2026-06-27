package middleware

import (
	"net/http"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	platformmw "github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx/middleware"
)

// InternalAuth protects /internal/v1 routes with the platform service token.
func InternalAuth(cfg *config.Config) func(http.Handler) http.Handler {
	if cfg == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return platformmw.InternalServiceAuth(*cfg)
}
