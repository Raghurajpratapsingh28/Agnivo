package events_test

import (
	"context"
	"testing"

	deployevents "github.com/agnivo/agnivo/packages/application/deploy/events"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/stretchr/testify/assert"
)

func TestPublishWithoutBus(t *testing.T) {
	p := deployevents.NewPublisher(nil, "test", nil)
	err := p.Publish(context.Background(), deployevents.DeploymentStarted, deployevents.Meta{
		ExecutionID: "e1", DeploymentID: "d1", OrgID: "o1", ProjectID: "p1",
	}, nil)
	assert.NoError(t, err)
}

func TestPublishWithBus(t *testing.T) {
	ctx := context.Background()
	bus := events.NewInMemory(ctx, events.Config{})
	defer func() { _ = bus.Close(ctx) }()
	p := deployevents.NewPublisher(bus, "deployer", nil)
	err := p.Publish(ctx, deployevents.ImagePulled, deployevents.Meta{
		ExecutionID: "e1", DeploymentID: "d1", OrgID: "o1", ProjectID: "p1",
		CorrelationID: "corr-1",
	}, map[string]string{"digest": "sha256:abc"})
	assert.NoError(t, err)
}
