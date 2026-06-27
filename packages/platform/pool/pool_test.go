package pool_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/pool"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPoolRunsAllTasks(t *testing.T) {
	var done int64
	p := pool.New(context.Background(), pool.Options{Workers: 4, QueueSize: 8})
	for i := 0; i < 100; i++ {
		require.NoError(t, p.Submit(func(context.Context) error {
			atomic.AddInt64(&done, 1)
			return nil
		}))
	}
	p.Close()
	assert.Equal(t, int64(100), atomic.LoadInt64(&done))
}

func TestPoolReportsErrors(t *testing.T) {
	var errCount int64
	p := pool.New(context.Background(), pool.Options{
		Workers: 2,
		OnError: func(error) { atomic.AddInt64(&errCount, 1) },
	})
	for i := 0; i < 10; i++ {
		_ = p.Submit(func(context.Context) error { return assertErr })
	}
	p.Close()
	assert.Equal(t, int64(10), atomic.LoadInt64(&errCount))
}

func TestSubmitAfterCloseFails(t *testing.T) {
	p := pool.New(context.Background(), pool.Options{Workers: 1})
	p.Close()
	err := p.Submit(func(context.Context) error { return nil })
	require.Error(t, err)
}

func TestStopCancels(t *testing.T) {
	p := pool.New(context.Background(), pool.Options{Workers: 1, QueueSize: 1})
	_ = p.Submit(func(ctx context.Context) error {
		time.Sleep(10 * time.Millisecond)
		return nil
	})
	p.Stop()
	// After Stop, submit must fail because the context is canceled.
	err := p.Submit(func(context.Context) error { return nil })
	require.Error(t, err)
}

var assertErr = errTest("boom")

type errTest string

func (e errTest) Error() string { return string(e) }
