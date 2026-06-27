package middleware

import (
	"crypto/subtle"
	"net/http"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// BearerToken requires Authorization: Bearer <token> using constant-time comparison.
// When token is empty the middleware is a no-op (development convenience).
func BearerToken(token string) func(http.Handler) http.Handler {
	if token == "" {
		return func(next http.Handler) http.Handler { return next }
	}
	expected := []byte(token)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			auth := r.Header.Get("Authorization")
			if !strings.HasPrefix(auth, "Bearer ") {
				dto.Error(w, r, errors.Unauthenticated("missing bearer token"))
				return
			}
			got := []byte(strings.TrimPrefix(auth, "Bearer "))
			if subtle.ConstantTimeCompare(got, expected) != 1 {
				dto.Error(w, r, errors.Unauthenticated("invalid bearer token"))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// InternalServiceAuth protects internal APIs with the configured service token.
// In production, requests are rejected when the token is unset.
func InternalServiceAuth(cfg config.Config) func(http.Handler) http.Handler {
	token := cfg.Security.InternalServiceToken
	if token != "" {
		return BearerToken(token)
	}
	if cfg.App.Environment.IsProduction() {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				dto.Error(w, r, errors.Unauthenticated("internal service token not configured"))
			})
		}
	}
	return func(next http.Handler) http.Handler { return next }
}
