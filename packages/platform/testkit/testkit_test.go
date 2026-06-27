package testkit_test

import (
	"context"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"github.com/agnivo/agnivo/packages/platform/testkit"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFakeBusRecordsEvents(t *testing.T) {
	bus := testkit.NewFakeBus(context.Background())
	defer func() { _ = bus.Close(context.Background()) }()

	e, err := events.New(context.Background(), "user.created", map[string]string{"id": "1"})
	require.NoError(t, err)
	require.NoError(t, bus.Publish(context.Background(), e))

	sync := bus.SyncEvents()
	require.Len(t, sync, 1)
	assert.Equal(t, "user.created", sync[0].Name)
}

func TestFakeJobQueueLifecycle(t *testing.T) {
	q := testkit.NewFakeJobQueue()
	ctx := context.Background()

	job, err := q.Enqueue(ctx, "builds", "compile", map[string]string{"ref": "main"}, jobs.EnqueueOptions{Priority: 1})
	require.NoError(t, err)

	claimed, err := q.Dequeue(ctx, "builds", "w1", 1, time.Minute)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, job.ID, claimed[0].ID)

	require.NoError(t, q.Complete(ctx, job.ID))
	assert.Equal(t, jobs.StatusSucceeded, q.All()[0].Status)
}

func TestFakeJobQueueIdempotency(t *testing.T) {
	q := testkit.NewFakeJobQueue()
	ctx := context.Background()
	opts := jobs.EnqueueOptions{IdempotencyKey: "k1"}
	a, _ := q.Enqueue(ctx, "q", "t", nil, opts)
	b, _ := q.Enqueue(ctx, "q", "t", nil, opts)
	assert.Equal(t, a.ID, b.ID)
}

func TestFactoryHelpers(t *testing.T) {
	assert.NotEmpty(t, testkit.RandomString(8))
	assert.Contains(t, testkit.RandomEmail(), "@test.local")
	assert.NotEmpty(t, testkit.RandomSlug())
	assert.NotEmpty(t, testkit.RandomUUID())
}

func TestNewSuite(t *testing.T) {
	s := testkit.NewSuite(t, time.Second)
	assert.NotNil(t, s.Context)
}
