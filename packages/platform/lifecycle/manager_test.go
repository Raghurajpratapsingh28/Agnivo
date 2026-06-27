package lifecycle_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/agnivo/agnivo/packages/platform/lifecycle"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestManager_RunsAndStopsInReverse(t *testing.T) {
	m := lifecycle.New(zap.NewNop(), 2*time.Second)

	var order []string
	m.AddHook(lifecycle.Hook{
		Name:  "first",
		Start: func(context.Context) error { order = append(order, "start-first"); return nil },
		Stop:  func(context.Context) error { order = append(order, "stop-first"); return nil },
	})
	m.AddHook(lifecycle.Hook{
		Name:  "second",
		Start: func(context.Context) error { order = append(order, "start-second"); return nil },
		Stop:  func(context.Context) error { order = append(order, "stop-second"); return nil },
	})

	var ran atomic.Bool
	m.AddRunner(lifecycle.Runner{Name: "runner", Run: func(ctx context.Context) error {
		ran.Store(true)
		<-ctx.Done()
		return ctx.Err()
	}})

	ctx, cancel := context.WithCancel(context.Background())
	go func() { time.Sleep(100 * time.Millisecond); cancel() }()

	require.NoError(t, m.Run(ctx))
	require.True(t, ran.Load())
	// Start in order, stop in reverse.
	require.Equal(t, []string{"start-first", "start-second", "stop-second", "stop-first"}, order)
}
