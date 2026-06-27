// Package retry provides context-aware retry execution with pluggable backoff
// strategies. It honors context cancellation, supports a retryability predicate
// so non-transient failures fail fast, and adds full jitter to avoid
// thundering-herd retries across many callers.
package retry

import (
	"context"
	"math"
	"math/rand"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Backoff computes the delay before the nth attempt (1-indexed). Implementations
// must be safe for concurrent use.
type Backoff interface {
	// Delay returns the wait duration before attempt number n (n >= 1).
	Delay(attempt int) time.Duration
}

// Exponential is an exponential backoff with optional full jitter and a ceiling.
type Exponential struct {
	// Base is the delay for the first retry.
	Base time.Duration
	// Max caps the computed delay.
	Max time.Duration
	// Factor is the multiplier applied each attempt (defaults to 2 when <= 1).
	Factor float64
	// Jitter, when true, randomizes each delay in [0, computed] (full jitter).
	Jitter bool
}

// Delay implements Backoff with exponential growth, capping, and optional jitter.
func (e Exponential) Delay(attempt int) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	factor := e.Factor
	if factor <= 1 {
		factor = 2
	}
	base := e.Base
	if base <= 0 {
		base = 50 * time.Millisecond
	}
	d := float64(base) * math.Pow(factor, float64(attempt-1))
	if e.Max > 0 && d > float64(e.Max) {
		d = float64(e.Max)
	}
	if e.Jitter {
		// Full jitter: spreads retries uniformly to decorrelate clients.
		d = rand.Float64() * d
	}
	return time.Duration(d)
}

// Constant waits a fixed duration between attempts.
type Constant struct{ Interval time.Duration }

// Delay implements Backoff with a constant interval.
func (c Constant) Delay(int) time.Duration { return c.Interval }

// Options configures Do.
type Options struct {
	// MaxAttempts bounds the total number of calls (>= 1). Zero defaults to 3.
	MaxAttempts int
	// Backoff controls inter-attempt delays. Defaults to a sane Exponential.
	Backoff Backoff
	// Retryable decides whether an error warrants another attempt. When nil,
	// errors.IsRetryable is used so the error framework drives retry policy.
	Retryable func(error) bool
}

// Do invokes fn until it succeeds, the retryability predicate rejects the
// error, attempts are exhausted, or ctx is canceled. The last error is returned
// on failure. Successful calls return nil immediately.
func Do(ctx context.Context, opts Options, fn func(ctx context.Context) error) error {
	attempts := opts.MaxAttempts
	if attempts < 1 {
		attempts = 3
	}
	backoff := opts.Backoff
	if backoff == nil {
		backoff = Exponential{Base: 50 * time.Millisecond, Max: 5 * time.Second, Factor: 2, Jitter: true}
	}
	retryable := opts.Retryable
	if retryable == nil {
		retryable = errors.IsRetryable
	}

	var last error
	for attempt := 1; attempt <= attempts; attempt++ {
		if err := ctx.Err(); err != nil {
			return errors.Wrap(err, errors.CodeCanceled, "retry: context canceled")
		}
		last = fn(ctx)
		if last == nil {
			return nil
		}
		if attempt == attempts || !retryable(last) {
			break
		}
		if err := sleep(ctx, backoff.Delay(attempt)); err != nil {
			return err
		}
	}
	return last
}

// sleep waits for d or until ctx is canceled, whichever comes first.
func sleep(ctx context.Context, d time.Duration) error {
	if d <= 0 {
		return nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return errors.Wrap(ctx.Err(), errors.CodeCanceled, "retry: context canceled")
	case <-t.C:
		return nil
	}
}
