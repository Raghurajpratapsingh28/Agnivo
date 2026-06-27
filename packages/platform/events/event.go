// Package events provides an internal event bus for decoupling producers from
// consumers. It supports synchronous and asynchronous publishing, multiple
// subscribers per event, automatic retry with a dead-letter sink, and rich
// event metadata (IDs, correlation IDs, timestamps, versioning).
//
// Business logic depends only on the Bus interface and the Event type, never on
// a concrete transport. The default InMemory bus can be swapped for a durable
// transport (Redis Streams, NATS, Kafka) later without touching producers or
// consumers.
package events

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// Wildcard subscribes a handler to every event name.
const Wildcard = "*"

// Event is an immutable record of something that happened. Name is a
// dot-delimited type (e.g. "deployment.created"); Version allows the payload
// schema to evolve while preserving older consumers.
type Event struct {
	// ID uniquely identifies this event instance (for idempotent consumers).
	ID string `json:"id"`
	// Name is the event type, e.g. "deployment.created".
	Name string `json:"name"`
	// Version is the payload schema version (>= 1).
	Version int `json:"version"`
	// OccurredAt is when the event happened, in UTC.
	OccurredAt time.Time `json:"occurred_at"`
	// CorrelationID ties this event to a request or causal chain.
	CorrelationID string `json:"correlation_id,omitempty"`
	// Source identifies the emitting component (e.g. "api", "deployer").
	Source string `json:"source,omitempty"`
	// Payload is the JSON-encoded event body.
	Payload json.RawMessage `json:"payload,omitempty"`
	// Metadata carries arbitrary string annotations.
	Metadata map[string]string `json:"metadata,omitempty"`
}

// Option customizes a constructed Event.
type Option func(*Event)

// WithVersion sets the payload schema version.
func WithVersion(v int) Option { return func(e *Event) { e.Version = v } }

// WithSource sets the emitting component name.
func WithSource(s string) Option { return func(e *Event) { e.Source = s } }

// WithCorrelationID overrides the correlation ID (otherwise taken from context).
func WithCorrelationID(id string) Option { return func(e *Event) { e.CorrelationID = id } }

// WithMetadata merges annotations into the event.
func WithMetadata(md map[string]string) Option {
	return func(e *Event) {
		if e.Metadata == nil {
			e.Metadata = make(map[string]string, len(md))
		}
		for k, v := range md {
			e.Metadata[k] = v
		}
	}
}

// New constructs an Event from a name and a payload that is JSON-marshaled. It
// fills the ID, OccurredAt (UTC now), a default Version of 1, and inherits the
// correlation ID from ctx when present. Returns an error only if payload cannot
// be marshaled.
func New(ctx context.Context, name string, payload any, opts ...Option) (Event, error) {
	var raw json.RawMessage
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return Event{}, errors.Wrap(err, errors.CodeInvalidArgument, "events: marshal payload")
		}
		raw = b
	}
	e := Event{
		ID:            idx.NewUUID(),
		Name:          name,
		Version:       1,
		OccurredAt:    time.Now().UTC(),
		CorrelationID: logger.CorrelationID(ctx),
		Payload:       raw,
	}
	for _, fn := range opts {
		fn(&e)
	}
	return e, nil
}

// Decode unmarshals the event payload into dst.
func (e Event) Decode(dst any) error {
	if len(e.Payload) == 0 {
		return errors.New(errors.CodeInvalidArgument, "events: empty payload")
	}
	if err := json.Unmarshal(e.Payload, dst); err != nil {
		return errors.Wrap(err, errors.CodeInvalidArgument, "events: decode payload")
	}
	return nil
}

// Handler consumes events. Returning an error triggers retry and, once retries
// are exhausted, dead-lettering.
type Handler interface {
	Handle(ctx context.Context, e Event) error
}

// HandlerFunc adapts a function to the Handler interface.
type HandlerFunc func(ctx context.Context, e Event) error

// Handle implements Handler.
func (f HandlerFunc) Handle(ctx context.Context, e Event) error { return f(ctx, e) }

// Bus is the transport-agnostic publish/subscribe surface that business logic
// depends on. Implementations must be safe for concurrent use.
type Bus interface {
	// Subscribe registers handler for an event name (or Wildcard for all).
	Subscribe(name string, handler Handler)
	// Publish delivers e to all matching handlers synchronously, returning the
	// combined error of any that failed after retries.
	Publish(ctx context.Context, e Event) error
	// PublishAsync enqueues e for background delivery and returns immediately.
	// Delivery failures are retried and dead-lettered, never returned here.
	PublishAsync(ctx context.Context, e Event) error
	// Close stops async delivery, draining in-flight work, and releases
	// resources.
	Close(ctx context.Context) error
}
