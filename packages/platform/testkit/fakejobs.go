package testkit

import (
	"context"
	"encoding/json"
	"sort"
	"sync"
	"time"

	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/jobs"
)

// FakeJobQueue is an in-memory job queue for unit tests. It mirrors the core
// enqueue/dequeue/complete/fail semantics of jobs.Queue without PostgreSQL.
type FakeJobQueue struct {
	mu    sync.Mutex
	jobs  map[string]jobs.Job
	order []string
}

// NewFakeJobQueue creates an empty fake queue.
func NewFakeJobQueue() *FakeJobQueue {
	return &FakeJobQueue{jobs: make(map[string]jobs.Job)}
}

// Enqueue inserts a job onto queue.
func (q *FakeJobQueue) Enqueue(_ context.Context, queue, jobType string, payload any, opts jobs.EnqueueOptions) (jobs.Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return jobs.Job{}, err
	}
	now := time.Now().UTC()
	key := opts.IdempotencyKey
	q.mu.Lock()
	if key != "" {
		for _, j := range q.jobs {
			if j.Queue == queue && j.IdempotencyKey != nil && *j.IdempotencyKey == key {
				q.mu.Unlock()
				return j, nil
			}
		}
	}
	maxAttempts := opts.MaxAttempts
	if maxAttempts <= 0 {
		maxAttempts = 25
	}
	availableAt := now
	if !opts.RunAt.IsZero() {
		availableAt = opts.RunAt.UTC()
	} else if opts.Delay > 0 {
		availableAt = now.Add(opts.Delay)
	}
	var keyPtr *string
	if key != "" {
		keyPtr = &key
	}
	j := jobs.Job{
		ID:             idx.NewUUID(),
		Queue:          queue,
		Type:           jobType,
		Payload:        raw,
		Priority:       opts.Priority,
		Status:         jobs.StatusPending,
		MaxAttempts:    maxAttempts,
		AvailableAt:    availableAt,
		IdempotencyKey: keyPtr,
		CreatedAt:      now,
		UpdatedAt:      now,
	}
	q.jobs[j.ID] = j
	q.order = append(q.order, j.ID)
	q.mu.Unlock()
	return j, nil
}

// Dequeue claims up to limit eligible jobs.
func (q *FakeJobQueue) Dequeue(_ context.Context, queue, workerID string, limit int, visibility time.Duration) ([]jobs.Job, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	now := time.Now().UTC()
	var out []jobs.Job
	for _, id := range q.order {
		if len(out) >= limit {
			break
		}
		j := q.jobs[id]
		if j.Queue != queue {
			continue
		}
		eligible := j.Status == jobs.StatusPending && !j.AvailableAt.After(now)
		if j.Status == jobs.StatusRunning && j.LeasedUntil != nil && j.LeasedUntil.Before(now) {
			eligible = true
		}
		if !eligible {
			continue
		}
		until := now.Add(visibility)
		j.Status = jobs.StatusRunning
		j.Attempts++
		j.LeasedUntil = &until
		j.LeasedBy = &workerID
		j.UpdatedAt = now
		q.jobs[id] = j
		out = append(out, j)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Priority != out[j].Priority {
			return out[i].Priority > out[j].Priority
		}
		return out[i].AvailableAt.Before(out[j].AvailableAt)
	})
	return out, nil
}

// Complete marks a job succeeded.
func (q *FakeJobQueue) Complete(_ context.Context, jobID string) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[jobID]
	if !ok {
		return errNotFound("job not found")
	}
	j.Status = jobs.StatusSucceeded
	j.LeasedUntil = nil
	j.LeasedBy = nil
	j.UpdatedAt = time.Now().UTC()
	q.jobs[jobID] = j
	return nil
}

// Fail reschedules or dead-letters a job.
func (q *FakeJobQueue) Fail(_ context.Context, jobID string, cause error, backoff time.Duration) (jobs.Status, error) {
	q.mu.Lock()
	defer q.mu.Unlock()
	j, ok := q.jobs[jobID]
	if !ok {
		return "", errNotFound("job not found")
	}
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	j.LastError = &msg
	j.LeasedUntil = nil
	j.LeasedBy = nil
	j.UpdatedAt = time.Now().UTC()
	if j.Attempts >= j.MaxAttempts {
		j.Status = jobs.StatusDead
	} else {
		j.Status = jobs.StatusPending
		j.AvailableAt = time.Now().UTC().Add(backoff)
	}
	q.jobs[jobID] = j
	return j.Status, nil
}

// All returns every job in insertion order.
func (q *FakeJobQueue) All() []jobs.Job {
	q.mu.Lock()
	defer q.mu.Unlock()
	out := make([]jobs.Job, 0, len(q.order))
	for _, id := range q.order {
		out = append(out, q.jobs[id])
	}
	return out
}

type errNotFound string

func (e errNotFound) Error() string { return string(e) }
