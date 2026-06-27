// Package timex provides time helpers and a Clock abstraction so time-dependent
// code can be tested deterministically. Production code uses the real clock;
// tests inject a fixed or controllable clock.
package timex

import (
	"sync"
	"time"
)

// Clock abstracts the current time so it can be faked in tests.
type Clock interface {
	// Now returns the current time.
	Now() time.Time
	// Since returns the time elapsed since t.
	Since(t time.Time) time.Duration
}

// realClock delegates to the standard library.
type realClock struct{}

func (realClock) Now() time.Time                  { return time.Now() }
func (realClock) Since(t time.Time) time.Duration { return time.Since(t) }

// System is the production clock backed by the standard library.
var System Clock = realClock{}

// FakeClock is a controllable Clock for tests. It is safe for concurrent use.
type FakeClock struct {
	mu sync.Mutex
	t  time.Time
}

// NewFakeClock returns a FakeClock anchored at start.
func NewFakeClock(start time.Time) *FakeClock { return &FakeClock{t: start} }

// Now returns the fake clock's current time.
func (c *FakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.t
}

// Since returns the elapsed time relative to the fake clock's current time.
func (c *FakeClock) Since(t time.Time) time.Duration { return c.Now().Sub(t) }

// Advance moves the fake clock forward by d.
func (c *FakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.t = c.t.Add(d)
}

// NowUTC returns the current time in UTC, the canonical storage timezone.
func NowUTC() time.Time { return time.Now().UTC() }

// MaxTime returns the later of a and b.
func MaxTime(a, b time.Time) time.Time {
	if a.After(b) {
		return a
	}
	return b
}

// MinTime returns the earlier of a and b.
func MinTime(a, b time.Time) time.Time {
	if a.Before(b) {
		return a
	}
	return b
}

// Clamp returns t bounded to the inclusive [min, max] range.
func Clamp(t, min, max time.Time) time.Time {
	if t.Before(min) {
		return min
	}
	if t.After(max) {
		return max
	}
	return t
}

// Truncate rounds t down to a multiple of d in UTC, useful for time bucketing.
func Truncate(t time.Time, d time.Duration) time.Time { return t.UTC().Truncate(d) }
