package metrics

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
)

// Middleware instruments HTTP handlers with request count, duration, and
// in-flight gauges. Route labels use the chi route pattern to bound cardinality.
func (r *Registry) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, req.ProtoMajor)

		method := req.Method
		r.httpInflight.WithLabelValues(method, "in_flight").Inc()
		defer r.httpInflight.WithLabelValues(method, "in_flight").Dec()

		next.ServeHTTP(ww, req)

		route := routePattern(req)
		status := strconv.Itoa(ww.Status())
		r.httpRequests.WithLabelValues(method, route, status).Inc()
		r.httpDuration.WithLabelValues(method, route, status).Observe(time.Since(start).Seconds())
	})
}

func routePattern(req *http.Request) string {
	if rctx := chi.RouteContext(req.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return "unmatched"
}
