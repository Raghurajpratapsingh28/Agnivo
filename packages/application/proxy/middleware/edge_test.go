package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agnivo/agnivo/packages/application/proxy/middleware"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestEdgeSecurityHeaders(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.EdgeSecurityHeaders(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	assert.Equal(t, "nosniff", rec.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "SAMEORIGIN", rec.Header().Get("X-Frame-Options"))
	assert.NotEmpty(t, rec.Header().Get("Strict-Transport-Security"))
}

func TestEdgeRequestID_GeneratesID(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.EdgeRequestID(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)

	assert.NotEmpty(t, rec.Header().Get("X-Request-ID"))
	assert.NotEmpty(t, rec.Header().Get("X-Correlation-ID"))
}

func TestEdgeRequestID_HonorsInbound(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.EdgeRequestID(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", "test-id-123")
	h.ServeHTTP(rec, req)

	assert.Equal(t, "test-id-123", rec.Header().Get("X-Request-ID"))
}

func TestWebSocketUpgrade_NonWS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.WebSocketUpgrade(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	h.ServeHTTP(rec, req)
	// No Upgrade header should be set for non-WS requests.
	assert.Empty(t, rec.Header().Get("Upgrade"))
}

func TestWebSocketUpgrade_WS(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.WebSocketUpgrade(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	req.Header.Set("Upgrade", "websocket")
	req.Header.Set("Connection", "Upgrade")
	h.ServeHTTP(rec, req)
	assert.Equal(t, "websocket", rec.Header().Get("Upgrade"))
}

func TestRealIP_Middleware(t *testing.T) {
	var capturedIP string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedIP = middleware.ClientIP(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.RealIP(nil)(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.RemoteAddr = "127.0.0.1:54321"
	req.Header.Set("X-Forwarded-For", "203.0.113.10")
	h.ServeHTTP(rec, req)
	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "203.0.113.10", capturedIP)
}

func TestInMemoryRateLimit_AllowsUnderLimit(t *testing.T) {
	calls := 0
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls++
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.InMemoryRateLimit(10, 1000)(next)
	for i := 0; i < 5; i++ {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = "192.0.2.1:1000"
		h.ServeHTTP(rec, req)
		assert.Equal(t, http.StatusOK, rec.Code)
	}
	assert.Equal(t, 5, calls)
}

func TestStreamingSupport_Header(t *testing.T) {
	next := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	h := middleware.StreamingSupport(next)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/stream", nil)
	h.ServeHTTP(rec, req)
	assert.Equal(t, "no", rec.Header().Get("X-Accel-Buffering"))
}
