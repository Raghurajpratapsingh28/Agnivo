package health_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agnivo/agnivo/packages/platform/health"
	"github.com/stretchr/testify/require"
)

func TestReady_AllHealthy(t *testing.T) {
	r := health.New()
	r.Register("db", func(context.Context) error { return nil })

	rec := httptest.NewRecorder()
	r.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	require.Equal(t, http.StatusOK, rec.Code)
	require.Contains(t, rec.Body.String(), `"status":"ok"`)
}

func TestReady_Degraded(t *testing.T) {
	r := health.New()
	r.Register("db", func(context.Context) error { return errors.New("down") })

	rec := httptest.NewRecorder()
	r.Ready(rec, httptest.NewRequest(http.MethodGet, "/health/ready", nil))

	require.Equal(t, http.StatusServiceUnavailable, rec.Code)
	require.Contains(t, rec.Body.String(), "unhealthy")
}

func TestLive(t *testing.T) {
	r := health.New()
	rec := httptest.NewRecorder()
	r.Live(rec, httptest.NewRequest(http.MethodGet, "/health/live", nil))
	require.Equal(t, http.StatusOK, rec.Code)
}
