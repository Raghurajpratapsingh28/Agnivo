package events_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type payload struct {
	Name string `json:"name"`
}

func TestNewEventFillsMetadata(t *testing.T) {
	e, err := events.New(context.Background(), "deployment.created", payload{Name: "web"},
		events.WithSource("api"), events.WithVersion(2))
	require.NoError(t, err)
	assert.NotEmpty(t, e.ID)
	assert.Equal(t, "deployment.created", e.Name)
	assert.Equal(t, 2, e.Version)
	assert.Equal(t, "api", e.Source)
	assert.False(t, e.OccurredAt.IsZero())

	var p payload
	require.NoError(t, e.Decode(&p))
	assert.Equal(t, "web", p.Name)
}

func TestPublishSyncFanOut(t *testing.T) {
	bus := events.NewInMemory(context.Background(), events.Config{})
	defer func() { _ = bus.Close(context.Background()) }()

	var a, b int32
	bus.Subscribe("user.created", events.HandlerFunc(func(context.Context, events.Event) error {
		atomic.AddInt32(&a, 1)
		return nil
	}))
	bus.Subscribe(events.Wildcard, events.HandlerFunc(func(context.Context, events.Event) error {
		atomic.AddInt32(&b, 1)
		return nil
	}))

	e, _ := events.New(context.Background(), "user.created", nil)
	require.NoError(t, bus.Publish(context.Background(), e))
	assert.Equal(t, int32(1), atomic.LoadInt32(&a))
	assert.Equal(t, int32(1), atomic.LoadInt32(&b)) // wildcard also fired
}

func TestPublishRetriesThenDeadLetters(t *testing.T) {
	dl := &captureDL{}
	bus := events.NewInMemory(context.Background(), events.Config{
		MaxAttempts: 3,
		Backoff:     retry.Constant{Interval: time.Millisecond},
		DeadLetter:  dl,
	})
	defer func() { _ = bus.Close(context.Background()) }()

	var calls int32
	bus.Subscribe("flaky", events.HandlerFunc(func(context.Context, events.Event) error {
		atomic.AddInt32(&calls, 1)
		return errors.Unavailable("nope")
	}))

	e, _ := events.New(context.Background(), "flaky", nil)
	err := bus.Publish(context.Background(), e)
	require.Error(t, err)
	assert.Equal(t, int32(3), atomic.LoadInt32(&calls)) // retried to exhaustion
	assert.Equal(t, 1, dl.count())
}

func TestPublishStopsOnNonRetryable(t *testing.T) {
	bus := events.NewInMemory(context.Background(), events.Config{MaxAttempts: 5})
	defer func() { _ = bus.Close(context.Background()) }()

	var calls int32
	bus.Subscribe("bad", events.HandlerFunc(func(context.Context, events.Event) error {
		atomic.AddInt32(&calls, 1)
		return errors.InvalidArgument("permanent")
	}))

	e, _ := events.New(context.Background(), "bad", nil)
	require.Error(t, bus.Publish(context.Background(), e))
	assert.Equal(t, int32(1), atomic.LoadInt32(&calls)) // invalid_argument is not retried
}

func TestPublishAsyncDelivers(t *testing.T) {
	bus := events.NewInMemory(context.Background(), events.Config{Workers: 4})

	var done sync.WaitGroup
	done.Add(1)
	bus.Subscribe("async", events.HandlerFunc(func(context.Context, events.Event) error {
		done.Done()
		return nil
	}))

	e, _ := events.New(context.Background(), "async", nil)
	require.NoError(t, bus.PublishAsync(context.Background(), e))

	waitTimeout(t, &done, time.Second)
	require.NoError(t, bus.Close(context.Background()))
}

type captureDL struct {
	mu sync.Mutex
	n  int
}

func (d *captureDL) Dead(context.Context, events.Event, string, error) {
	d.mu.Lock()
	d.n++
	d.mu.Unlock()
}
func (d *captureDL) count() int {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.n
}

func waitTimeout(t *testing.T, wg *sync.WaitGroup, d time.Duration) {
	t.Helper()
	done := make(chan struct{})
	go func() { wg.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(d):
		t.Fatal("timed out waiting for async delivery")
	}
}
