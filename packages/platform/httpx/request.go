package httpx

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/go-chi/chi/v5"
)

// QueryString returns the first value for key from the query string, or "".
func QueryString(r *http.Request, key string) string { return r.URL.Query().Get(key) }

// QueryInt parses key as an integer, returning (0, false) when absent or invalid.
func QueryInt(r *http.Request, key string) (int, bool) {
	s := QueryString(r, key)
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0, false
	}
	return n, true
}

// QueryBool parses key as a boolean ("true", "1", "false", "0").
func QueryBool(r *http.Request, key string) (bool, bool) {
	s := strings.ToLower(QueryString(r, key))
	switch s {
	case "true", "1", "yes":
		return true, true
	case "false", "0", "no":
		return false, true
	default:
		return false, false
	}
}

// QueryStrings returns all values for a repeated query parameter.
func QueryStrings(r *http.Request, key string) []string { return r.URL.Query()[key] }

// PathParam returns a chi URL path parameter.
func PathParam(r *http.Request, key string) string { return chi.URLParam(r, key) }

// RequirePathParam returns a chi URL path parameter or CodeInvalidArgument when empty.
func RequirePathParam(r *http.Request, key string) (string, error) {
	v := PathParam(r, key)
	if v == "" {
		return "", errors.Newf(errors.CodeInvalidArgument, "missing path parameter %q", key)
	}
	return v, nil
}

// Header returns the first value for a request header.
func Header(r *http.Request, key string) string { return r.Header.Get(key) }

// ClientIP returns a best-effort client IP, honoring X-Forwarded-For and
// X-Real-IP when present (typical behind a reverse proxy).
func ClientIP(r *http.Request) string {
	if xff := Header(r, "X-Forwarded-For"); xff != "" {
		if i := strings.IndexByte(xff, ','); i >= 0 {
			return strings.TrimSpace(xff[:i])
		}
		return strings.TrimSpace(xff)
	}
	if xri := Header(r, "X-Real-IP"); xri != "" {
		return strings.TrimSpace(xri)
	}
	host := r.RemoteAddr
	if i := strings.LastIndexByte(host, ':'); i >= 0 {
		return host[:i]
	}
	return host
}
