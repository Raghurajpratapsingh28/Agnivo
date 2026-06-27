package middleware

import (
	"net/http"

	idhttp "github.com/agnivo/agnivo/packages/application/identity/http"
)

// StreamAuth validates JWT/API-key authentication for /stream/v1 routes.
func StreamAuth(mw *idhttp.Middleware) func(http.Handler) http.Handler {
	if mw == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	// Authenticate then require an authenticated principal for all stream routes.
	return func(next http.Handler) http.Handler {
		return mw.RequireAuth(mw.Authenticate(next))
	}
}
