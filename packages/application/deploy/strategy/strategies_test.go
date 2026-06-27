package strategy_test

import (
	"context"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/runtime"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/strategy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type mockDriver struct {
	created []string
	started []string
}

func (m *mockDriver) Create(_ context.Context, deploymentID string, cfg model.RuntimeConfig) (runtime.ContainerInfo, error) {
	m.created = append(m.created, deploymentID)
	return runtime.ContainerInfo{ID: "ctr-" + deploymentID, Image: cfg.Image, Port: cfg.HostPort}, nil
}

func (m *mockDriver) Start(_ context.Context, containerID string) error {
	m.started = append(m.started, containerID)
	return nil
}

func (m *mockDriver) Stop(context.Context, string, time.Duration) error { return nil }
func (m *mockDriver) Remove(context.Context, string) error              { return nil }
func (m *mockDriver) Inspect(context.Context, string) (runtime.ContainerInfo, error) {
	return runtime.ContainerInfo{}, nil
}

func TestRollingDeploy(t *testing.T) {
	drv := &mockDriver{}
	reg := strategy.NewRegistry(model.StrategyRolling)
	sctx := strategy.Context{
		DeploymentID: "dep-1",
		Runtime: model.RuntimeConfig{
			Image: "nginx:latest", Labels: map[string]string{},
		},
		Placement: strategy.Placement{Host: "127.0.0.1", Port: 30001},
	}
	res, err := reg.Get(model.StrategyRolling).Deploy(context.Background(), sctx, drv)
	require.NoError(t, err)
	assert.Equal(t, "ctr-dep-1", res.Container.ID)
	assert.Equal(t, "127.0.0.1", res.Container.Host)
	assert.Equal(t, []string{"dep-1"}, drv.created)
	assert.Equal(t, []string{"ctr-dep-1"}, drv.started)
}

func TestCanarySetsLabel(t *testing.T) {
	drv := &mockDriver{}
	reg := strategy.NewRegistry(model.StrategyRolling)
	sctx := strategy.Context{
		DeploymentID: "dep-2",
		Runtime: model.RuntimeConfig{
			Image: "app:v1", Labels: map[string]string{},
		},
		Placement: strategy.Placement{Host: "127.0.0.1", Port: 30002},
	}
	_, err := reg.Get(model.StrategyCanary).Deploy(context.Background(), sctx, drv)
	require.NoError(t, err)
}
