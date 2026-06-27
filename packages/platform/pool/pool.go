// Package pool provides a bounded worker pool for fan-out concurrency with
// backpressure. It is a thin, allocation-conscious layer over goroutines and a
// buffered task channel, used wherever the platform needs to cap concurrency
// (parallel deployments, batch processing) without spawning unbounded
// goroutines.
package pool

import (
	"context"
	"sync"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Task is a unit of work executed by a pool worker.
type Task func(ctx context.Context) error

// Pool runs Tasks across a fixed number of workers. Submit applies backpressure
// when the queue is full, so producers cannot outrun consumers unboundedly.
type Pool struct {
	tasks   chan Task
	wg      sync.WaitGroup
	ctx     context.Context
	cancel  context.CancelFunc
	onError func(error)

	mu     sync.RWMutex // guards closed; serializes Submit against Close
	closed bool
}

// Options configures a Pool.
type Options struct {
	// Workers is the number of concurrent workers (>= 1). Zero defaults to 1.
	Workers int
	// QueueSize is the buffered task capacity. Zero makes Submit synchronous
	// with worker availability.
	QueueSize int
	// OnError is invoked for each task error. When nil, errors are dropped; use
	// Group when you need to collect them.
	OnError func(error)
}

// New starts a Pool bound to ctx. When ctx is canceled, workers drain in-flight
// tasks and stop accepting new ones. Call Close to wait for completion.
func New(ctx context.Context, opts Options) *Pool {
	workers := opts.Workers
	if workers < 1 {
		workers = 1
	}
	cctx, cancel := context.WithCancel(ctx)
	p := &Pool{
		tasks:   make(chan Task, opts.QueueSize),
		ctx:     cctx,
		cancel:  cancel,
		onError: opts.OnError,
	}
	p.wg.Add(workers)
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *Pool) worker() {
	defer p.wg.Done()
	for {
		select {
		case <-p.ctx.Done():
			return
		case task, ok := <-p.tasks:
			if !ok {
				return
			}
			if err := task(p.ctx); err != nil && p.onError != nil {
				p.onError(err)
			}
		}
	}
}

// Submit enqueues task, blocking when the queue is full until space is
// available or the pool's context is canceled. It returns an error if the pool
// is shutting down. The read lock makes sends safe against a concurrent Close
// without serializing submitters with one another.
func (p *Pool) Submit(task Task) error {
	p.mu.RLock()
	defer p.mu.RUnlock()
	if p.closed {
		return errors.New(errors.CodeUnavailable, "pool: closed")
	}
	select {
	case <-p.ctx.Done():
		return errors.Wrap(p.ctx.Err(), errors.CodeUnavailable, "pool: shutting down")
	case p.tasks <- task:
		return nil
	}
}

// Close stops accepting tasks, waits for all queued and in-flight tasks to
// finish, and releases resources. It is safe to call multiple times.
func (p *Pool) Close() {
	p.mu.Lock()
	if p.closed {
		p.mu.Unlock()
		return
	}
	p.closed = true
	close(p.tasks)
	p.mu.Unlock()

	p.wg.Wait()
	p.cancel()
}

// Stop cancels the pool immediately, abandoning queued tasks, and waits for
// in-flight tasks to observe cancellation.
func (p *Pool) Stop() {
	p.cancel()
	p.wg.Wait()
}
