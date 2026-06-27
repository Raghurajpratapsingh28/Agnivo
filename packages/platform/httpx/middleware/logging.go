package middleware

import (
	"net/http"
	"time"

	"github.com/agnivo/agnivo/packages/platform/logger"
	"github.com/go-chi/chi/v5/middleware"
	"go.uber.org/zap"
)

// Logger injects the request-scoped logger into the context and emits one
// structured access log per request with method, route, status, and latency.
func Logger(base *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := logger.Into(r.Context(), base)
			r = r.WithContext(ctx)

			ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
			start := time.Now()

			next.ServeHTTP(ww, r)

			logger.From(r.Context()).Info("http_request",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", ww.Status()),
				zap.Int("bytes", ww.BytesWritten()),
				zap.Duration("latency", time.Since(start)),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
			)
		})
	}
}
