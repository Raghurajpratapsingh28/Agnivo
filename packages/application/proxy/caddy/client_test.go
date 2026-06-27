package caddy_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/caddy"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func newMockCaddy(t *testing.T) (*httptest.Server, *caddy.Client) {
	t.Helper()
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Simulate Caddy @id-based route lookup (404 → not found, triggers append).
	mux.HandleFunc("/id/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Method == http.MethodDelete {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("null"))
			return
		}
		w.WriteHeader(http.StatusNotFound)
	})

	// Simulate append to routes array.
	mux.HandleFunc("/config/apps/http/servers/main/routes", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			w.WriteHeader(http.StatusOK)
			return
		}
		// GET — return empty routes list.
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]any{})
	})

	// TLS automate endpoint.
	mux.HandleFunc("/config/apps/tls/certificates/automate", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			_ = json.NewEncoder(w).Encode([]string{})
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Config root for Ping.
	mux.HandleFunc("/config/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{})
	})

	// Load endpoint.
	mux.HandleFunc("/load", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	client := caddy.NewClient(srv.URL, zap.NewNop())
	return srv, client
}

func TestPing(t *testing.T) {
	_, client := newMockCaddy(t)
	require.NoError(t, client.Ping(context.Background()))
}

func TestUpsertRoute_NewRoute(t *testing.T) {
	_, client := newMockCaddy(t)
	err := client.UpsertRoute(context.Background(), model.CaddyRouteConfig{
		Hostname:       "example.com",
		Upstream:       "localhost:3000",
		TLSEnabled:     true,
		HTTPSRedirect:  true,
		TimeoutSeconds: 30,
		MaxRetries:     3,
	})
	require.NoError(t, err)
}

func TestDeleteRoute(t *testing.T) {
	_, client := newMockCaddy(t)
	// Should succeed (returns 200 or 404 both treated as ok).
	require.NoError(t, client.DeleteRoute(context.Background(), "example.com"))
}

func TestAutomateCert(t *testing.T) {
	_, client := newMockCaddy(t)
	require.NoError(t, client.AutomateCert(context.Background(), "example.com"))
	// Calling again with same hostname is idempotent.
	require.NoError(t, client.AutomateCert(context.Background(), "example.com"))
}

func TestLoadConfig(t *testing.T) {
	_, client := newMockCaddy(t)
	routes := []model.CaddyRouteConfig{
		{Hostname: "app1.example.com", Upstream: "10.0.0.1:8080", TLSEnabled: true},
		{Hostname: "app2.example.com", Upstream: "10.0.0.2:8080", TLSEnabled: true},
	}
	tlsHostnames := []string{"app1.example.com", "app2.example.com"}
	require.NoError(t, client.LoadConfig(context.Background(), routes, tlsHostnames))
}

func TestUpsertRoute_WithCanary(t *testing.T) {
	_, client := newMockCaddy(t)
	err := client.UpsertRoute(context.Background(), model.CaddyRouteConfig{
		Hostname:       "canary.example.com",
		Upstream:       "localhost:3001",
		CanaryWeight:   20,
		CanaryUpstream: "localhost:3000",
		TLSEnabled:     true,
	})
	require.NoError(t, err)
}

func TestRevokeCert(t *testing.T) {
	_, client := newMockCaddy(t)
	require.NoError(t, client.AutomateCert(context.Background(), "todelete.com"))
	require.NoError(t, client.RevokeCert(context.Background(), "todelete.com"))
}

func TestUpsertRoute_AddHeaders(t *testing.T) {
	_, client := newMockCaddy(t)
	err := client.UpsertRoute(context.Background(), model.CaddyRouteConfig{
		Hostname: "headers.example.com",
		Upstream: "localhost:4000",
		AddHeaders: map[string]string{
			"X-Custom-Header": "value1",
		},
	})
	assert.NoError(t, err)
}
