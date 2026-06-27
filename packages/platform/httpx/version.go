package httpx

import (
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
)

// VersionHeader is the standard header for explicit API version selection.
const VersionHeader = "X-API-Version"

// VersionFromRequest resolves the requested API version from, in order:
//  1. X-API-Version header
//  2. Accept-Version header (legacy alias)
//  3. chi route parameter "version" (e.g. /v1/...)
//
// Returns "" when no version is specified.
func VersionFromRequest(r *http.Request) string {
	if v := strings.TrimSpace(Header(r, VersionHeader)); v != "" {
		return normalizeVersion(v)
	}
	if v := strings.TrimSpace(Header(r, "Accept-Version")); v != "" {
		return normalizeVersion(v)
	}
	if v := chi.URLParam(r, "version"); v != "" {
		return normalizeVersion(v)
	}
	return ""
}

// RequireVersion returns the requested version or CodeInvalidArgument when absent.
func RequireVersion(r *http.Request) (string, error) {
	v := VersionFromRequest(r)
	if v == "" {
		return "", ErrBadRequest("API version is required")
	}
	return v, nil
}

// VersionMiddleware validates that the resolved version is in allowed, or that
// no version was specified (permitting unversioned routes). When a version is
// present but not allowed, the middleware responds with 400.
func VersionMiddleware(allowed ...string) func(http.Handler) http.Handler {
	set := make(map[string]struct{}, len(allowed))
	for _, v := range allowed {
		set[normalizeVersion(v)] = struct{}{}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			v := VersionFromRequest(r)
			if v == "" {
				next.ServeHTTP(w, r)
				return
			}
			if _, ok := set[v]; !ok {
				Error(w, r, ErrBadRequest("unsupported API version: "+v))
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

// normalizeVersion strips a leading "v" and lowercases, so "V1" and "1" both
// become "1".
func normalizeVersion(v string) string {
	v = strings.TrimSpace(strings.ToLower(v))
	return strings.TrimPrefix(v, "v")
}
