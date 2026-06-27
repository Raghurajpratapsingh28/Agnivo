package streaming_test

import (
	"context"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/streaming"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestHub_SubscribeAndUnsubscribe(t *testing.T) {
	// Hub without Redis (dispatch from internal publish only).
	hub := streaming.NewHub(nil, zap.NewNop())

	ctx, cancel := context.WithCancel(context.Background())
	sub := hub.Subscribe(ctx, streaming.ChannelForProject("proj-1"), 10)
	require.NotNil(t, sub)

	stats := hub.Stats()
	assert.Equal(t, int64(1), stats.SSEConnections)

	cancel()
	// Allow the goroutine to unsubscribe.
	time.Sleep(10 * time.Millisecond)

	stats = hub.Stats()
	assert.Equal(t, int64(0), stats.SSEConnections)
}

func TestHub_MultipleSubscribers(t *testing.T) {
	hub := streaming.NewHub(nil, zap.NewNop())

	ctx1, c1 := context.WithCancel(context.Background())
	ctx2, c2 := context.WithCancel(context.Background())
	defer c1()
	defer c2()

	hub.Subscribe(ctx1, streaming.ChannelForProject("p1"), 8)
	hub.Subscribe(ctx2, streaming.ChannelForProject("p2"), 8)

	stats := hub.Stats()
	assert.Equal(t, int64(2), stats.SSEConnections)
}

func TestHub_Stats_Empty(t *testing.T) {
	hub := streaming.NewHub(nil, zap.NewNop())
	stats := hub.Stats()
	assert.Equal(t, int64(0), stats.SSEConnections)
	assert.Equal(t, int64(0), stats.ActiveSubscriptions)
}

func TestChannelHelpers(t *testing.T) {
	assert.Contains(t, streaming.ChannelForDeployment("dep-1"), "dep-1")
	assert.Contains(t, streaming.ChannelForProject("proj-1"), "proj-1")
	assert.Contains(t, streaming.ChannelForOrg("org-1"), "org-1")
}

func TestHub_Heartbeat_NoSubscribers(t *testing.T) {
	hub := streaming.NewHub(nil, zap.NewNop())
	// Heartbeat with no subscribers should not panic.
	assert.NotPanics(t, func() {
		hub.Heartbeat(context.Background())
	})
}

func TestStreamMessage_Fields(t *testing.T) {
	msg := model.StreamMessage{
		Channel:      "proxy:project:proj-1",
		EventType:    "proxy.route.created",
		OrgID:        "org-1",
		ProjectID:    "proj-1",
		DeploymentID: "dep-1",
	}
	assert.Equal(t, "proxy.route.created", msg.EventType)
	assert.Equal(t, "proj-1", msg.ProjectID)
}
