package events

import (
	"context"
	"sync"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/ctxx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/pool"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/retry"
	"go.uber.org/zap"
)

// DeadLetter receives events whose delivery failed after all retries. A durable
// implementation might persist them to a table or queue for inspection and
// replay; the default logs them.
type DeadLetter interface {
	Dead(ctx context.Context, e Event, handlerName string, err error)
}

// Config tunes an InMemory bus.
type Config struct {
	// Workers is the async delivery concurrency (>= 1). Defaults to runtime-ish 8.
	Workers int
	// QueueSize buffers async events before Submit blocks. Defaults to 1024.
	QueueSize int
	// MaxAttempts bounds delivery attempts per handler (>= 1). Defaults to 3.
	MaxAttempts int
	// Backoff controls inter-attempt delay. Defaults to a short exponential.
	Backoff retry.Backoff
	// AsyncTimeout bounds the lifetime of a detached async delivery. Defaults
	// to 30s.
	AsyncTimeout time.Duration
	// DeadLetter sinks exhausted deliveries. Defaults to a logging sink.
	DeadLetter DeadLetter
	// Logger is used for warnings and the default dead-letter sink.
	Logger *zap.Logger
}

func (c *Config) withDefaults() {
	if c.Workers < 1 {
		c.Workers = 8
	}
	if c.QueueSize < 1 {
		c.QueueSize = 1024
	}
	if c.MaxAttempts < 1 {
		c.MaxAttempts = 3
	}
	if c.Backoff == nil {
		c.Backoff = retry.Exponential{Base: 20 * time.Millisecond, Max: time.Second, Factor: 2, Jitter: true}
	}
	if c.AsyncTimeout <= 0 {
		c.AsyncTimeout = 30 * time.Second
	}
	if c.Logger == nil {
		c.Logger = zap.NewNop()
	}
	if c.DeadLetter == nil {
		c.DeadLetter = logDeadLetter{log: c.Logger}
	}
}

// namedHandler pairs a handler with a stable name for logging and dead-lettering.
type namedHandler struct {
	name    string
	handler Handler
}

// InMemory is a process-local Bus implementation. It is the default transport
// and is fully featured (multiple subscribers, retry, dead-letter, async). It
// can be replaced by a durable transport without changing producers/consumers.
type InMemory struct {
	cfg  Config
	mu   sync.RWMutex
	subs map[string][]namedHandler
	pool *pool.Pool
}

// compile-time assertion.
var _ Bus = (*InMemory)(nil)

// NewInMemory constructs an InMemory bus bound to ctx; async delivery stops when
// ctx is canceled or Close is called.
func NewInMemory(ctx context.Context, cfg Config) *InMemory {
	cfg.withDefaults()
	b := &InMemory{cfg: cfg, subs: make(map[string][]namedHandler)}
	b.pool = pool.New(ctx, pool.Options{
		Workers:   cfg.Workers,
		QueueSize: cfg.QueueSize,
		OnError: func(err error) {
			cfg.Logger.Warn("event delivery error", zap.Error(err))
		},
	})
	return b
}

// Subscribe registers handler for name (or Wildcard). The handler's concrete
// type name is used for logging and dead-lettering.
func (b *InMemory) Subscribe(name string, handler Handler) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.subs[name] = append(b.subs[name], namedHandler{name: handlerName(handler), handler: handler})
}

// matching returns a snapshot of handlers for an event name plus wildcard subs.
func (b *InMemory) matching(name string) []namedHandler {
	b.mu.RLock()
	defer b.mu.RUnlock()
	exact := b.subs[name]
	wild := b.subs[Wildcard]
	out := make([]namedHandler, 0, len(exact)+len(wild))
	out = append(out, exact...)
	out = append(out, wild...)
	return out
}

// Publish delivers e synchronously, retrying each handler independently and
// dead-lettering any that exhaust retries. It returns the joined errors of all
// failed handlers so the caller can react (or ignore) as appropriate.
func (b *InMemory) Publish(ctx context.Context, e Event) error {
	handlers := b.matching(e.Name)
	var errs []error
	for _, h := range handlers {
		if err := b.deliver(ctx, h, e); err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return errors.Join(errs...)
	}
	return nil
}

// PublishAsync enqueues delivery to each matching handler onto the worker pool.
// The originating context is detached (its values, e.g. correlation ID, are
// preserved) and bounded by AsyncTimeout, so async work survives request
// completion but cannot leak indefinitely.
func (b *InMemory) PublishAsync(_ context.Context, e Event) error {
	handlers := b.matching(e.Name)
	for _, h := range handlers {
		h := h
		err := b.pool.Submit(func(workerCtx context.Context) error {
			dctx, cancel := ctxx.WithTimeoutCause(injectCorrelation(workerCtx, e), b.cfg.AsyncTimeout)
			defer cancel()
			return b.deliver(dctx, h, e)
		})
		if err != nil {
			return errors.Wrap(err, errors.CodeUnavailable, "events: enqueue async delivery")
		}
	}
	return nil
}

// deliver invokes one handler with retry, dead-lettering on final failure.
func (b *InMemory) deliver(ctx context.Context, h namedHandler, e Event) error {
	err := retry.Do(ctx, retry.Options{
		MaxAttempts: b.cfg.MaxAttempts,
		Backoff:     b.cfg.Backoff,
		// Event handlers are retried regardless of code unless the error is
		// explicitly non-retryable, since transient handler failures are common.
		Retryable: func(err error) bool { return !errors.IsCode(err, errors.CodeInvalidArgument) },
	}, func(ctx context.Context) error {
		return h.handler.Handle(ctx, e)
	})
	if err != nil {
		b.cfg.DeadLetter.Dead(ctx, e, h.name, err)
		return errors.Wrapf(err, errors.CodeOf(err), "events: handler %q failed for %q", h.name, e.Name)
	}
	return nil
}

// Close stops async delivery, draining queued events.
func (b *InMemory) Close(_ context.Context) error {
	b.pool.Close()
	return nil
}

// logDeadLetter is the default dead-letter sink; it logs at error level.
type logDeadLetter struct{ log *zap.Logger }

func (d logDeadLetter) Dead(_ context.Context, e Event, handlerName string, err error) {
	d.log.Error("event dead-lettered",
		zap.String("event_id", e.ID),
		zap.String("event_name", e.Name),
		zap.String("handler", handlerName),
		zap.String("correlation_id", e.CorrelationID),
		zap.Error(err),
	)
}
