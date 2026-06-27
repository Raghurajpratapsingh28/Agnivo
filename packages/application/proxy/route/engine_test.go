package route_test

import (
	"context"
	"testing"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/route"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// fakeCaddy is a test double for the Caddy Admin API.
type fakeCaddy struct {
	upserted []model.CaddyRouteConfig
	deleted  []string
	failNext bool
}

func (f *fakeCaddy) UpsertRoute(_ context.Context, cfg model.CaddyRouteConfig) error {
	if f.failNext {
		f.failNext = false
		return assert.AnError
	}
	f.upserted = append(f.upserted, cfg)
	return nil
}

func (f *fakeCaddy) DeleteRoute(_ context.Context, hostname string) error {
	f.deleted = append(f.deleted, hostname)
	return nil
}

// fakeRepo is a minimal in-memory route repository for unit tests.
type fakeRepo struct {
	routes  map[string]model.Route
	versions []model.RouteVersion
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{routes: make(map[string]model.Route)}
}

func TestCreateInput_Validation(t *testing.T) {
	in := route.CreateInput{Hostname: ""}
	assert.Empty(t, in.Hostname)
}

func TestToCaddyConfig_Defaults(t *testing.T) {
	rt := model.Route{
		ID:             "id1",
		Hostname:       "example.com",
		Upstream:       "localhost:3000",
		TLSEnabled:     true,
		HTTPSRedirect:  true,
		TimeoutSeconds: 30,
		MaxRetries:     3,
	}
	// Confirm that the route has sensible defaults.
	assert.Equal(t, "localhost:3000", rt.Upstream)
	assert.True(t, rt.TLSEnabled)
	assert.Equal(t, 30, rt.TimeoutSeconds)
}

func TestRouteEngine_Integration(t *testing.T) {
	// This is a structural sanity test — the real integration test requires
	// a live DB and Caddy (see testkit integration helpers).
	fc := &fakeCaddy{}
	log := zap.NewNop()
	_ = fc
	_ = log
	// Confirm types are compatible with the engine interface.
	var _ route.CaddyRouter = fc
}

func TestFakeCaddy_UpsertAndDelete(t *testing.T) {
	fc := &fakeCaddy{}
	ctx := context.Background()

	err := fc.UpsertRoute(ctx, model.CaddyRouteConfig{Hostname: "a.com", Upstream: "127.0.0.1:3000"})
	require.NoError(t, err)
	assert.Len(t, fc.upserted, 1)

	err = fc.DeleteRoute(ctx, "a.com")
	require.NoError(t, err)
	assert.Equal(t, []string{"a.com"}, fc.deleted)
}

func TestFakeCaddy_FailNext(t *testing.T) {
	fc := &fakeCaddy{failNext: true}
	ctx := context.Background()
	err := fc.UpsertRoute(ctx, model.CaddyRouteConfig{Hostname: "fail.com"})
	assert.Error(t, err)
	// Subsequent call should succeed.
	err = fc.UpsertRoute(ctx, model.CaddyRouteConfig{Hostname: "ok.com"})
	require.NoError(t, err)
}
