// Package ctxx provides context helpers: detaching a context from its parent's
// cancellation while preserving its values, and merging cancellation signals.
// These patterns recur when launching background work derived from a
// request-scoped context.
package ctxx

import (
	"context"
	"time"
)

// detached carries the values of a parent context but never inherits its
// deadline or cancellation. This lets background work (event publishing, async
// jobs) outlive the request that triggered it while keeping correlation IDs.
type detached struct{ vals context.Context }

func (detached) Deadline() (time.Time, bool)       { return time.Time{}, false }
func (detached) Done() <-chan struct{}             { return nil }
func (detached) Err() error                        { return nil }
func (d detached) Value(key any) any               { return d.vals.Value(key) }

// Detach returns a context that retains parent's values but is never canceled
// by parent. The caller is responsible for bounding its lifetime (e.g. with
// WithTimeout) to avoid leaks.
func Detach(parent context.Context) context.Context { return detached{vals: parent} }

// WithTimeoutCause is a thin wrapper that detaches from the parent's
// cancellation and applies a fresh timeout, preserving values. Use it for
// fire-and-forget work that must complete within a bound regardless of whether
// the originating request was canceled.
func WithTimeoutCause(parent context.Context, timeout time.Duration) (context.Context, context.CancelFunc) {
	return context.WithTimeout(Detach(parent), timeout)
}

// Merge returns a context that is canceled when either a or b is canceled, or
// when the returned cancel func is called. The cancel func must always be
// invoked to release the watcher goroutine. Values resolve from a.
func Merge(a, b context.Context) (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(a)
	stop := make(chan struct{})
	go func() {
		select {
		case <-b.Done():
			cancel()
		case <-ctx.Done():
		case <-stop:
		}
	}()
	return ctx, func() {
		close(stop)
		cancel()
	}
}
