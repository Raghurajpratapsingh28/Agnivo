package jobs

import (
	"context"
	"sync"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/retry"
	"go.uber.org/zap"
)

// HandlerFunc processes a single job. Returning nil completes the job; a
// non-nil error triggers retry-or-dead-letter via Queue.Fail.
type HandlerFunc func(ctx context.Context, job Job) error

// WorkerConfig tunes a Worker.
type WorkerConfig struct {
	// Queue is the queue name to consume.
	Queue string
	// Concurrency is the number of jobs processed in parallel. Defaults to 1.
	Concurrency int
	// BatchSize is how many jobs to claim per poll. Defaults to Concurrency.
	BatchSize int
	// PollInterval is the idle wait between empty polls. Defaults to 1s.
	PollInterval time.Duration
	// Visibility is the lease duration; a job not completed within it is
	// reclaimed. Defaults to 30s.
	Visibility time.Duration
	// HeartbeatInterval extends the lease for long-running jobs. Defaults to
	// Visibility/3. Set to 0 to disable heartbeating.
	HeartbeatInterval time.Duration
	// RetryBackoff computes the delay before a failed job becomes available
	// again, indexed by attempt. Defaults to a capped exponential.
	RetryBackoff retry.Backoff
	// Logger receives operational logs.
	Logger *zap.Logger
}

func (c *WorkerConfig) withDefaults() {
	if c.Concurrency < 1 {
		c.Concurrency = 1
	}
	if c.BatchSize < 1 {
		c.BatchSize = c.Concurrency
	}
	if c.PollInterval <= 0 {
		c.PollInterval = time.Second
	}
	if c.Visibility <= 0 {
		c.Visibility = 30 * time.Second
	}
	if c.HeartbeatInterval == 0 {
		c.HeartbeatInterval = c.Visibility / 3
	}
	if c.RetryBackoff == nil {
		c.RetryBackoff = retry.Exponential{Base: time.Second, Max: 10 * time.Minute, Factor: 2, Jitter: true}
	}
	if c.Logger == nil {
		c.Logger = zap.NewNop()
	}
}

// Worker consumes a queue, dispatching jobs to registered handlers across a
// bounded pool of goroutines. It is designed to be registered as a lifecycle
// runner (App.AddRunner): Run blocks until ctx is canceled, then drains.
type Worker struct {
	queue    *Queue
	cfg      WorkerConfig
	id       string
	handlers map[string]HandlerFunc
}

// NewWorker constructs a Worker. Register handlers with Handle before Run.
func NewWorker(queue *Queue, cfg WorkerConfig) *Worker {
	cfg.withDefaults()
	return &Worker{
		queue:    queue,
		cfg:      cfg,
		id:       idx.Prefixed("worker", 8),
		handlers: make(map[string]HandlerFunc),
	}
}

// Handle registers fn for jobs of the given type. It is not safe to call after
// Run has started.
func (w *Worker) Handle(jobType string, fn HandlerFunc) { w.handlers[jobType] = fn }

// Run polls the queue and processes jobs until ctx is canceled. Each poll claims
// a batch and processes it concurrently up to Concurrency; when a poll yields no
// work it sleeps PollInterval. On cancellation it waits for in-flight jobs to
// finish (their leases protect them from double-processing regardless).
func (w *Worker) Run(ctx context.Context) error {
	w.cfg.Logger.Info("job worker started",
		zap.String("worker_id", w.id), zap.String("queue", w.cfg.Queue),
		zap.Int("concurrency", w.cfg.Concurrency))

	sem := make(chan struct{}, w.cfg.Concurrency)
	var wg sync.WaitGroup

	for {
		select {
		case <-ctx.Done():
			wg.Wait()
			w.cfg.Logger.Info("job worker stopped", zap.String("worker_id", w.id))
			return ctx.Err()
		default:
		}

		batch, err := w.queue.Dequeue(ctx, w.cfg.Queue, w.id, w.cfg.BatchSize, w.cfg.Visibility)
		if err != nil {
			if ctx.Err() != nil {
				wg.Wait()
				return ctx.Err()
			}
			w.cfg.Logger.Warn("dequeue failed", zap.Error(err))
			if !sleep(ctx, w.cfg.PollInterval) {
				wg.Wait()
				return ctx.Err()
			}
			continue
		}
		if len(batch) == 0 {
			if !sleep(ctx, w.cfg.PollInterval) {
				wg.Wait()
				return ctx.Err()
			}
			continue
		}

		for _, job := range batch {
			job := job
			sem <- struct{}{}
			wg.Add(1)
			go func() {
				defer wg.Done()
				defer func() { <-sem }()
				w.process(ctx, job)
			}()
		}
	}
}

// process runs one job: dispatches to its handler under a heartbeat, then
// completes or fails it. Panics in handlers are converted to failures so one
// bad job never crashes the worker.
func (w *Worker) process(ctx context.Context, job Job) {
	start := time.Now()
	handler, ok := w.handlers[job.Type]
	if !ok {
		// Unknown type: fail without retry-storms by dead-lettering quickly.
		_, _ = w.queue.Fail(ctx, job.ID, errors.Newf(errors.CodeFailedPrecond, "jobs: no handler for type %q", job.Type), w.backoff(job))
		w.queue.metrics.observe(job.Type, "no_handler", time.Since(start).Seconds())
		return
	}

	hbCtx, stopHB := context.WithCancel(ctx)
	if w.cfg.HeartbeatInterval > 0 {
		go w.heartbeat(hbCtx, job.ID)
	}

	err := w.safeInvoke(ctx, handler, job)
	stopHB()

	if err != nil {
		status, ferr := w.queue.Fail(ctx, job.ID, err, w.backoff(job))
		if ferr != nil {
			w.cfg.Logger.Error("failed to mark job failed", zap.String("job_id", job.ID), zap.Error(ferr))
		}
		w.cfg.Logger.Warn("job failed",
			zap.String("job_id", job.ID), zap.String("type", job.Type),
			zap.Int("attempt", job.Attempts), zap.String("result", string(status)), zap.Error(err))
		w.queue.metrics.observe(job.Type, "failed", time.Since(start).Seconds())
		return
	}

	if cerr := w.queue.Complete(ctx, job.ID); cerr != nil {
		w.cfg.Logger.Error("failed to mark job complete", zap.String("job_id", job.ID), zap.Error(cerr))
	}
	w.queue.metrics.observe(job.Type, "succeeded", time.Since(start).Seconds())
}

// safeInvoke runs the handler, recovering panics into errors.
func (w *Worker) safeInvoke(ctx context.Context, handler HandlerFunc, job Job) (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.Newf(errors.CodeInternal, "jobs: handler panic: %v", r)
		}
	}()
	return handler(ctx, job)
}

// heartbeat extends the job's lease until hbCtx is canceled (job done) or the
// lease is lost.
func (w *Worker) heartbeat(ctx context.Context, jobID string) {
	t := time.NewTicker(w.cfg.HeartbeatInterval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := w.queue.Heartbeat(ctx, jobID, w.id, w.cfg.Visibility); err != nil {
				// Lease lost: stop heartbeating; the job may be reclaimed.
				return
			}
		}
	}
}

// backoff computes the retry delay for a job based on its attempt count.
func (w *Worker) backoff(job Job) time.Duration {
	return w.cfg.RetryBackoff.Delay(job.Attempts)
}

// sleep waits for d or ctx cancellation; it reports false if canceled.
func sleep(ctx context.Context, d time.Duration) bool {
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
