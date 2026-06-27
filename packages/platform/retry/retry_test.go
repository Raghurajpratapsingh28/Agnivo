package retry_test

import (
	"context"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/retry"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDoSucceedsAfterRetries(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		Backoff:     retry.Constant{Interval: time.Millisecond},
	}, func(context.Context) error {
		calls++
		if calls < 3 {
			return errors.Unavailable("not yet")
		}
		return nil
	})
	require.NoError(t, err)
	assert.Equal(t, 3, calls)
}

func TestDoStopsOnNonRetryable(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 5,
		Backoff:     retry.Constant{Interval: time.Millisecond},
	}, func(context.Context) error {
		calls++
		return errors.NotFound("gone") // not retryable by default
	})
	require.Error(t, err)
	assert.Equal(t, 1, calls)
}

func TestDoExhaustsAttempts(t *testing.T) {
	calls := 0
	err := retry.Do(context.Background(), retry.Options{
		MaxAttempts: 3,
		Backoff:     retry.Constant{Interval: time.Millisecond},
	}, func(context.Context) error {
		calls++
		return errors.Unavailable("down")
	})
	require.Error(t, err)
	assert.Equal(t, 3, calls)
}

func TestDoHonorsContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := retry.Do(ctx, retry.Options{MaxAttempts: 3}, func(context.Context) error {
		return errors.Unavailable("x")
	})
	require.Error(t, err)
	assert.Equal(t, errors.CodeCanceled, errors.CodeOf(err))
}

func TestExponentialBackoffCaps(t *testing.T) {
	b := retry.Exponential{Base: 100 * time.Millisecond, Max: 400 * time.Millisecond, Factor: 2}
	assert.Equal(t, 100*time.Millisecond, b.Delay(1))
	assert.Equal(t, 200*time.Millisecond, b.Delay(2))
	assert.Equal(t, 400*time.Millisecond, b.Delay(3))
	assert.Equal(t, 400*time.Millisecond, b.Delay(10)) // capped
}
