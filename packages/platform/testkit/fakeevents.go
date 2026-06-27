package testkit

import (
	"context"
	"sync"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
)

// FakeBus is an in-memory event bus for tests that records every published
// event for assertions. It wraps events.InMemory and is safe for concurrent use.
type FakeBus struct {
	inner *events.InMemory
	mu    sync.Mutex
	sync  []events.Event
	async []events.Event
}

// NewFakeBus constructs a FakeBus bound to ctx.
func NewFakeBus(ctx context.Context) *FakeBus {
	return &FakeBus{inner: events.NewInMemory(ctx, events.Config{})}
}

// Subscribe delegates to the underlying bus.
func (b *FakeBus) Subscribe(name string, handler events.Handler) { b.inner.Subscribe(name, handler) }

// Publish records and synchronously delivers the event.
func (b *FakeBus) Publish(ctx context.Context, e events.Event) error {
	b.record(&b.sync, e)
	return b.inner.Publish(ctx, e)
}

// PublishAsync records and asynchronously delivers the event.
func (b *FakeBus) PublishAsync(ctx context.Context, e events.Event) error {
	b.record(&b.async, e)
	return b.inner.PublishAsync(ctx, e)
}

// Close stops async delivery.
func (b *FakeBus) Close(ctx context.Context) error { return b.inner.Close(ctx) }

// SyncEvents returns a copy of synchronously published events.
func (b *FakeBus) SyncEvents() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]events.Event(nil), b.sync...)
}

// AsyncEvents returns a copy of asynchronously published events.
func (b *FakeBus) AsyncEvents() []events.Event {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]events.Event(nil), b.async...)
}

func (b *FakeBus) record(dst *[]events.Event, e events.Event) {
	b.mu.Lock()
	defer b.mu.Unlock()
	*dst = append(*dst, e)
}
