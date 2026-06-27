// Package worker provides a small, shared poll-loop primitive for queue-consuming
// executables. Job claiming and processing live in feature packages; this only
// owns the cancellation-aware scheduling of work.
package worker

import (
	"context"
	"time"

	"go.uber.org/zap"
)

// Loop invokes fn every interval until the context is cancelled. fn errors are
// logged and the loop continues, so a transient failure never kills the worker.
func Loop(ctx context.Context, log *zap.Logger, name string, interval time.Duration, fn func(ctx context.Context) error) error {
	log.Info("worker loop started", zap.String("worker", name), zap.Duration("interval", interval))
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			log.Info("worker loop stopped", zap.String("worker", name))
			return ctx.Err()
		case <-ticker.C:
			if err := fn(ctx); err != nil {
				log.Error("worker tick failed", zap.String("worker", name), zap.Error(err))
			}
		}
	}
}
