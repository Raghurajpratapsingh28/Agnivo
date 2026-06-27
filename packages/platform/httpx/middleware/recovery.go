package middleware

import (
	"net/http"
	"runtime/debug"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// Recovery converts panics into 500 responses, logging the stack trace with the
// request-scoped logger so the failure is correlated and traceable.
func Recovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.From(r.Context()).Error("panic recovered",
					zap.Any("panic", rec),
					zap.ByteString("stack", debug.Stack()),
				)
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"error":{"code":"internal_error","message":"an unexpected error occurred"}}`))
			}
		}()
		next.ServeHTTP(w, r)
	})
}
