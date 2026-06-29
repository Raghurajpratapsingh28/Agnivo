package events_test

import (
	"context"
	"testing"

	buildevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/stretchr/testify/assert"
)

func TestPublishWithoutBus(t *testing.T) {
	p := buildevents.NewPublisher(nil, "test", nil)
	err := p.Publish(context.Background(), buildevents.BuildStarted, buildevents.Meta{
		BuildID: "b1", DeploymentID: "d1", OrgID: "o1", ProjectID: "p1",
	}, nil)
	assert.NoError(t, err)
}

func TestPublishWithBus(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInMemory(ctx, events.Config{})
	defer func() { _ = bus.Close(ctx) }()
	p := buildevents.NewPublisher(bus, "builder", nil)
	err := p.Publish(ctx, buildevents.FrameworkDetected, buildevents.Meta{
		BuildID: "b1", DeploymentID: "d1", OrgID: "o1", ProjectID: "p1",
		CorrelationID: "corr-1",
	}, map[string]string{"framework": "nextjs"})
	assert.NoError(t, err)
}
