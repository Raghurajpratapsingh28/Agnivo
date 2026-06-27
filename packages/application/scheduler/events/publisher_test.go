package events_test

import (
	"context"
	"testing"

	schedevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/stretchr/testify/assert"
)

func TestPublishWithoutBus(t *testing.T) {
	p := schedevents.NewPublisher(nil, "test", nil)
	err := p.Publish(context.Background(), schedevents.PlacementRequested, schedevents.Meta{
		DeploymentID: "d1", OrgID: "o1", ProjectID: "p1",
	}, nil)
	assert.NoError(t, err)
}

func TestPublishWithBus(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInMemory(ctx, events.Config{})
	defer func() { _ = bus.Close(ctx) }()
	p := schedevents.NewPublisher(bus, "scheduler", nil)
	err := p.Publish(ctx, schedevents.ReservationCreated, schedevents.Meta{
		DeploymentID: "d1", CorrelationID: "c1",
	}, map[string]int{"port": 30001})
	assert.NoError(t, err)
}
