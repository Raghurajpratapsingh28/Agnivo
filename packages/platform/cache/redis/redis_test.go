package redis_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// dial connects to the Redis instance named by REDIS_TEST_URL, skipping the
// test when it is not set so unit suites run without external services.
func dial(t *testing.T) *redis.Client {
	t.Helper()
	url := os.Getenv("REDIS_TEST_URL")
	if url == "" {
		t.Skip("REDIS_TEST_URL not set; skipping Redis integration test")
	}
	cfg := &config.Config{Redis: config.Redis{Enabled: true, URL: url}}
	c, err := redis.New(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return c
}

func TestMetricsUnit(t *testing.T) {
	m := redis.NewMetrics("test")
	assert.NotEmpty(t, m.Collectors())
	assert.NotPanics(t, func() { m.ObserveCommand("get", 0.001, nil) })
	var nilM *redis.Metrics
	assert.NotPanics(t, func() { nilM.ObserveCommand("get", 0.001, nil) })
}

func TestLockLifecycle(t *testing.T) {
	c := dial(t)
	ctx := context.Background()
	key := "test:lock:" + time.Now().Format("150405.000")

	lock, err := c.Acquire(ctx, key, time.Second)
	require.NoError(t, err)

	// A second acquire must fail while the lock is held.
	_, err = c.Acquire(ctx, key, time.Second)
	assert.True(t, errors.Is(err, redis.ErrLockNotAcquired))

	require.NoError(t, lock.Extend(ctx, 2*time.Second))
	require.NoError(t, lock.Release(ctx))

	// After release the lock is free again.
	lock2, err := c.Acquire(ctx, key, time.Second)
	require.NoError(t, err)
	require.NoError(t, lock2.Release(ctx))
}

func TestTokenBucket(t *testing.T) {
	c := dial(t)
	ctx := context.Background()
	key := "test:rl:" + time.Now().Format("150405.000")

	bucket := c.NewTokenBucket(3, 1) // capacity 3, refill 1/s
	for i := 0; i < 3; i++ {
		res, err := bucket.Allow(ctx, key)
		require.NoError(t, err)
		assert.True(t, res.Allowed, "request %d should be allowed", i)
	}
	res, err := bucket.Allow(ctx, key)
	require.NoError(t, err)
	assert.False(t, res.Allowed)
	assert.Greater(t, res.RetryAfter, time.Duration(0))
}

func TestTTLHelpers(t *testing.T) {
	c := dial(t)
	ctx := context.Background()
	key := "test:ttl:" + time.Now().Format("150405.000")

	require.NoError(t, c.SetTTL(ctx, key, "v", time.Minute))
	v, ok, err := c.GetString(ctx, key)
	require.NoError(t, err)
	assert.True(t, ok)
	assert.Equal(t, "v", v)

	ttl, err := c.TTL(ctx, key)
	require.NoError(t, err)
	assert.Greater(t, ttl, time.Duration(0))

	n, err := c.Delete(ctx, key)
	require.NoError(t, err)
	assert.Equal(t, int64(1), n)

	_, ok, err = c.GetString(ctx, key)
	require.NoError(t, err)
	assert.False(t, ok)
}
