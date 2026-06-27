package events_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	opsevents "github.com/agnivo/agnivo/packages/application/ops/events"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPublisher_Async(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	bus := events.NewInMemory(ctx, events.Config{})

	var received atomic.Int32
	bus.Subscribe(opsevents.UsageUpdated, events.HandlerFunc(func(_ context.Context, _ events.Event) error {
		received.Add(1)
		return nil
	}))

	pub := opsevents.NewPublisher(bus, "test")
	err := pub.PublishAsync(ctx, opsevents.UsageUpdated,
		opsevents.Meta{OrgID: "org1", CorrelationID: "corr1"},
		map[string]string{"period": "2026-06-25"})
	require.NoError(t, err)

	// Give async delivery a moment.
	time.Sleep(100 * time.Millisecond)
	assert.Equal(t, int32(1), received.Load())
}

func TestEventTypes(t *testing.T) {
	types := []string{
		opsevents.UsageUpdated, opsevents.QuotaExceeded, opsevents.QuotaWarning,
		opsevents.ProjectSleeping, opsevents.ProjectAwakened,
		opsevents.BackupCompleted, opsevents.BackupFailed,
		opsevents.NotificationDelivered, opsevents.NotificationFailed,
		opsevents.SubscriptionRenewed, opsevents.InvoiceGenerated,
		opsevents.PlanChanged, opsevents.AnalyticsUpdated,
		opsevents.CleanupCompleted, opsevents.GarbageCollected,
	}
	for _, et := range types {
		assert.NotEmpty(t, et)
	}
}
