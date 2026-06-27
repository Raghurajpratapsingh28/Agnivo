// Package middleware contains reusable, business-agnostic HTTP middleware.
package middleware

import (
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/logger"
	"github.com/google/uuid"
)

// Header names for request correlation.
const (
	HeaderRequestID     = "X-Request-ID"
	HeaderCorrelationID = "X-Correlation-ID"
)

// RequestID assigns a unique ID per request (honoring an inbound X-Request-ID),
// stores it in the context, and echoes it on the response.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if id == "" {
			id = uuid.NewString()
		}
		ctx := logger.WithRequestID(r.Context(), id)
		w.Header().Set(HeaderRequestID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// CorrelationID propagates a cross-service correlation ID, defaulting to the
// request ID when absent so a single request is always traceable end-to-end.
func CorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderCorrelationID)
		if id == "" {
			id = logger.RequestID(r.Context())
		}
		if id == "" {
			id = uuid.NewString()
		}
		ctx := logger.WithCorrelationID(r.Context(), id)
		w.Header().Set(HeaderCorrelationID, id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}
