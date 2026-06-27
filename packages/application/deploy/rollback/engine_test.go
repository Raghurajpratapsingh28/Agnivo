package rollback_test

import (
	"context"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/deploy/model"
	"github.com/agnivo/agnivo/packages/application/deploy/rollback"
	"github.com/agnivo/agnivo/packages/application/deploy/runtime"
	"github.com/stretchr/testify/assert"
)

func TestShouldRollback(t *testing.T) {
	assert.True(t, rollback.ShouldRollback(rollback.TriggerHealthFailed))
	assert.True(t, rollback.ShouldRollback(rollback.TriggerContainerCrash))
	assert.False(t, rollback.ShouldRollback(rollback.TriggerImagePull))
	assert.False(t, rollback.ShouldRollback(rollback.TriggerManual))
}

func TestDrainContainers(t *testing.T) {
	drv := &mockDriver{}
	engine := rollback.NewEngine(nil, drv, nil)
	drained := engine.DrainContainers(t.Context(), []string{"c1", "c2"})
	assert.Equal(t, []string{"c1", "c2"}, drained)
	assert.Equal(t, []string{"c1", "c2"}, drv.stopped)
	assert.Equal(t, []string{"c1", "c2"}, drv.removed)
}

type mockDriver struct {
	stopped, removed []string
}

func (m *mockDriver) Create(context.Context, string, model.RuntimeConfig) (runtime.ContainerInfo, error) {
	return runtime.ContainerInfo{}, nil
}
func (m *mockDriver) Start(context.Context, string) error { return nil }
func (m *mockDriver) Stop(_ context.Context, id string, _ time.Duration) error {
	m.stopped = append(m.stopped, id)
	return nil
}
func (m *mockDriver) Remove(_ context.Context, id string) error {
	m.removed = append(m.removed, id)
	return nil
}
func (m *mockDriver) Inspect(context.Context, string) (runtime.ContainerInfo, error) {
	return runtime.ContainerInfo{}, nil
}
