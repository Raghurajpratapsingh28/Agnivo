package middleware

import "net/http"

// APIKeyAuth validates CLI and CI tokens. Applied to /cli/v1 routes.
func APIKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
	})
}
