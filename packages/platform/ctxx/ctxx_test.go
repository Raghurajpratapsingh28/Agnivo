package ctxx_test

import (
	"context"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/ctxx"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type k string

func TestDetachKeepsValuesDropsCancel(t *testing.T) {
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), k("id"), "abc"))
	d := ctxx.Detach(parent)
	cancel()
	// Parent canceled, but detached context is unaffected and keeps values.
	assert.NoError(t, d.Err())
	assert.Equal(t, "abc", d.Value(k("id")))
	_, hasDeadline := d.Deadline()
	assert.False(t, hasDeadline)
}

func TestWithTimeoutCause(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()
	ctx, c := ctxx.WithTimeoutCause(parent, 50*time.Millisecond)
	defer c()
	// Detached from parent cancellation; expires only on its own timeout.
	select {
	case <-ctx.Done():
		t.Fatal("should not be done yet")
	default:
	}
	time.Sleep(80 * time.Millisecond)
	assert.Error(t, ctx.Err())
}

func TestMergeCancelsOnEither(t *testing.T) {
	a := context.Background()
	b, cancelB := context.WithCancel(context.Background())
	merged, cancel := ctxx.Merge(a, b)
	defer cancel()
	cancelB()
	select {
	case <-merged.Done():
	case <-time.After(time.Second):
		require.Fail(t, "merged context should cancel when b cancels")
	}
}
