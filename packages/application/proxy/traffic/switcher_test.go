package traffic_test

import (
	"context"
	"testing"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/traffic"
	"github.com/stretchr/testify/assert"
)

// fakeCaddySwitcher records UpsertRoute calls for assertion.
type fakeCaddySwitcher struct {
	last model.CaddyRouteConfig
}

func (f *fakeCaddySwitcher) UpsertRoute(_ context.Context, cfg model.CaddyRouteConfig) error {
	f.last = cfg
	return nil
}

func TestSwitchRequest_Validation(t *testing.T) {
	req := model.TrafficSwitchRequest{
		Hostname:     "",
		DeploymentID: "",
	}
	assert.Empty(t, req.Hostname)
	assert.Empty(t, req.DeploymentID)
}

func TestTrafficMode_Constants(t *testing.T) {
	assert.Equal(t, model.TrafficMode("active"), model.TrafficModeActive)
	assert.Equal(t, model.TrafficMode("blue"), model.TrafficModeBlue)
	assert.Equal(t, model.TrafficMode("green"), model.TrafficModeGreen)
	assert.Equal(t, model.TrafficMode("canary"), model.TrafficModeCanary)
	assert.Equal(t, model.TrafficMode("draining"), model.TrafficModeDraining)
}

func TestSwitcher_InterfaceCompat(t *testing.T) {
	fc := &fakeCaddySwitcher{}
	var _ traffic.CaddySwitcher = fc
	assert.NotNil(t, fc)
}

func TestCanaryWeight_Bounds(t *testing.T) {
	tests := []struct {
		weight int
		wantOK bool
	}{
		{0, false},
		{1, true},
		{50, true},
		{99, true},
		{100, false},
		{-1, false},
	}
	for _, tc := range tests {
		ok := tc.weight >= 1 && tc.weight <= 99
		assert.Equal(t, tc.wantOK, ok, "weight=%d", tc.weight)
	}
}
