package middleware

import (
	"net/http"

	idhttp "github.com/Raghurajpratapsingh28/Agnivo/packages/application/identity/http"
)

// JWTAuth validates dashboard session tokens via the identity platform middleware.
// Wire with identity.Module.Middleware.Authenticate when mounting routes outside
// the identity router.
func JWTAuth(mw *idhttp.Middleware) func(http.Handler) http.Handler {
	if mw == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return mw.Authenticate
}

// RequireAuth ensures a user is authenticated.
func RequireAuth(mw *idhttp.Middleware) func(http.Handler) http.Handler {
	if mw == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return mw.RequireAuth
}
