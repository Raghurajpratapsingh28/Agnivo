package events_test

import (
	"context"
	"testing"

	rtevents "github.com/agnivo/agnivo/packages/application/runtimeagent/events"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/stretchr/testify/assert"
)

func TestPublishWithoutBus(t *testing.T) {
	p := rtevents.NewPublisher(nil, "test", nil)
	err := p.Publish(context.Background(), rtevents.ContainerStarted, rtevents.Meta{
		ContainerID: "c1", DeploymentID: "d1",
	}, nil)
	assert.NoError(t, err)
}

func TestPublishWithBus(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInMemory(ctx, events.Config{})
	defer func() { _ = bus.Close(ctx) }()
	p := rtevents.NewPublisher(bus, "runtime-agent", nil)
	err := p.Publish(ctx, rtevents.ImagePulled, rtevents.Meta{DeploymentID: "d1"}, map[string]string{"image": "app:v1"})
	assert.NoError(t, err)
}
