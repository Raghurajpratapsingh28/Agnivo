package middleware

import (
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/dto"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

const defaultMaxBodyBytes int64 = 10 << 20 // 10 MiB

// MaxBodyBytes rejects requests whose Content-Length exceeds limit.
// When limit <= 0 the platform default (10 MiB) is used.
func MaxBodyBytes(limit int64) func(http.Handler) http.Handler {
	if limit <= 0 {
		limit = defaultMaxBodyBytes
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.ContentLength > limit {
				dto.Error(w, r, errors.New(errors.CodeInvalidArgument, "request body too large"))
				return
			}
			r.Body = http.MaxBytesReader(w, r.Body, limit)
			next.ServeHTTP(w, r)
		})
	}
}
