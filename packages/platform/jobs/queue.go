package jobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Queue is the durable job store. Enqueue participates in the caller's
// transaction (via db.Conn), enabling a transactional outbox: a job is enqueued
// atomically with the business write that justifies it.
type Queue struct {
	db      *postgres.DB
	metrics *Metrics
}

// NewQueue constructs a Queue over the given database.
func NewQueue(db *postgres.DB, metrics *Metrics) *Queue {
	return &Queue{db: db, metrics: metrics}
}

const jobColumns = `id, queue, type, payload, priority, status, attempts, max_attempts,
	available_at, leased_until, leased_by, last_error, idempotency_key, created_at, updated_at`

// Enqueue inserts a job onto queue with the given type and payload. When an
// IdempotencyKey is supplied and a job with that (queue, key) already exists,
// the existing job is returned and no duplicate is created. Because it routes
// through db.Conn, enqueue joins any surrounding transaction.
func (q *Queue) Enqueue(ctx context.Context, queue, jobType string, payload any, opts EnqueueOptions) (Job, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return Job{}, errors.Wrap(err, errors.CodeInvalidArgument, "jobs: marshal payload")
	}
	now := time.Now().UTC()
	var keyPtr *string
	if opts.IdempotencyKey != "" {
		keyPtr = &opts.IdempotencyKey
	}

	// ON CONFLICT makes the insert idempotent on (queue, idempotency_key).
	const insert = `
INSERT INTO jobs (id, queue, type, payload, priority, status, max_attempts, available_at, idempotency_key)
VALUES ($1, $2, $3, $4, $5, 'pending', $6, $7, $8)
ON CONFLICT (queue, idempotency_key) WHERE idempotency_key IS NOT NULL
DO UPDATE SET updated_at = jobs.updated_at
RETURNING ` + jobColumns

	row := q.db.Conn(ctx).QueryRow(ctx, insert,
		idx.NewUUID(), queue, jobType, raw, opts.Priority, opts.maxAttempts(),
		opts.availableAt(now), keyPtr)

	job, err := scanJobRow(row)
	if err != nil {
		return Job{}, postgres.Translate(err, "jobs: enqueue")
	}
	q.metrics.incEnqueued(queue)
	return job, nil
}

// Dequeue atomically claims up to limit eligible jobs from queue for workerID,
// leasing them for the visibility duration. It uses FOR UPDATE SKIP LOCKED so
// concurrent workers never block one another or claim the same job. Jobs whose
// previous lease expired are reclaimed. The claim increments attempts and sets
// the lease; callers must Complete, Fail, or Heartbeat before the lease ends.
func (q *Queue) Dequeue(ctx context.Context, queue, workerID string, limit int, visibility time.Duration) ([]Job, error) {
	if limit <= 0 {
		limit = 1
	}
	const claim = `
UPDATE jobs SET
	status = 'running',
	attempts = attempts + 1,
	leased_until = now() + $3::interval,
	leased_by = $4,
	updated_at = now()
WHERE id IN (
	SELECT id FROM jobs
	WHERE queue = $1
	  AND (
	      (status = 'pending' AND available_at <= now())
	      OR (status = 'running' AND leased_until < now())
	  )
	ORDER BY priority DESC, available_at
	FOR UPDATE SKIP LOCKED
	LIMIT $2
)
RETURNING ` + jobColumns

	interval := visibilityInterval(visibility)
	rows, err := q.db.Conn(ctx).Query(ctx, claim, queue, limit, interval, workerID)
	if err != nil {
		return nil, postgres.Translate(err, "jobs: dequeue")
	}
	defer rows.Close()

	jobs, err := scanJobs(rows)
	if err != nil {
		return nil, postgres.Translate(err, "jobs: scan dequeued")
	}
	q.metrics.addDequeued(queue, len(jobs))
	return jobs, nil
}

// Complete marks a leased job as succeeded. Succeeded rows are retained so
// idempotency keys keep deduping; prune them with PurgeCompleted.
func (q *Queue) Complete(ctx context.Context, jobID string) error {
	const sql = `UPDATE jobs SET status='succeeded', leased_until=NULL, leased_by=NULL, last_error=NULL, updated_at=now() WHERE id=$1`
	tag, err := q.db.Conn(ctx).Exec(ctx, sql, jobID)
	if err != nil {
		return postgres.Translate(err, "jobs: complete")
	}
	if tag.RowsAffected() == 0 {
		return errors.New(errors.CodeNotFound, "jobs: job not found")
	}
	q.metrics.incCompleted()
	return nil
}

// Fail records a processing failure. If attempts remain, the job is rescheduled
// with exponential backoff (becoming pending again at a future available_at);
// otherwise it is dead-lettered (StatusDead). The provided cause is stored for
// diagnostics.
func (q *Queue) Fail(ctx context.Context, jobID string, cause error, backoff time.Duration) (Status, error) {
	msg := ""
	if cause != nil {
		msg = cause.Error()
	}
	// A single statement decides retry vs. dead-letter using the row's own
	// attempt counters, avoiding a read-modify-write race.
	const sql = `
UPDATE jobs SET
	status = CASE WHEN attempts >= max_attempts THEN 'dead' ELSE 'pending' END,
	available_at = CASE WHEN attempts >= max_attempts THEN available_at ELSE now() + $2::interval END,
	leased_until = NULL,
	leased_by = NULL,
	last_error = $3,
	updated_at = now()
WHERE id = $1
RETURNING status`

	var status Status
	row := q.db.Conn(ctx).QueryRow(ctx, sql, jobID, visibilityInterval(backoff), msg)
	if err := row.Scan(&status); err != nil {
		return "", postgres.Translate(err, "jobs: fail")
	}
	if status == StatusDead {
		q.metrics.incDead()
	} else {
		q.metrics.incRetried()
	}
	return status, nil
}

// Heartbeat extends the lease on a running job, signaling the worker is still
// alive. It returns CodeConflict if the job is no longer leased by workerID
// (e.g. its lease already expired and another worker reclaimed it).
func (q *Queue) Heartbeat(ctx context.Context, jobID, workerID string, extend time.Duration) error {
	const sql = `
UPDATE jobs SET leased_until = now() + $3::interval, updated_at = now()
WHERE id = $1 AND leased_by = $2 AND status = 'running'`
	tag, err := q.db.Conn(ctx).Exec(ctx, sql, jobID, workerID, visibilityInterval(extend))
	if err != nil {
		return postgres.Translate(err, "jobs: heartbeat")
	}
	if tag.RowsAffected() == 0 {
		return errors.New(errors.CodeConflict, "jobs: lease lost")
	}
	return nil
}

// PurgeCompleted deletes succeeded jobs older than the given age, reclaiming
// space. Run it periodically (e.g. from the cron executable).
func (q *Queue) PurgeCompleted(ctx context.Context, olderThan time.Duration) (int64, error) {
	const sql = `DELETE FROM jobs WHERE status='succeeded' AND updated_at < now() - $1::interval`
	tag, err := q.db.Conn(ctx).Exec(ctx, sql, visibilityInterval(olderThan))
	if err != nil {
		return 0, postgres.Translate(err, "jobs: purge")
	}
	return tag.RowsAffected(), nil
}

// Stats returns counts per status for a queue, for monitoring depth and health.
func (q *Queue) Stats(ctx context.Context, queue string) (map[Status]int64, error) {
	const sql = `SELECT status, count(*) FROM jobs WHERE queue=$1 GROUP BY status`
	rows, err := q.db.Conn(ctx).Query(ctx, sql, queue)
	if err != nil {
		return nil, postgres.Translate(err, "jobs: stats")
	}
	defer rows.Close()

	out := make(map[Status]int64)
	for rows.Next() {
		var s Status
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return nil, postgres.Translate(err, "jobs: scan stats")
		}
		out[s] = n
	}
	return out, postgres.Translate(rows.Err(), "jobs: iterate stats")
}

// visibilityInterval renders a duration as a Postgres interval string. A
// non-positive duration becomes "0 seconds".
func visibilityInterval(d time.Duration) string {
	if d <= 0 {
		return "0 seconds"
	}
	return d.String()
}

func scanJobRow(row pgx.Row) (Job, error) {
	var j Job
	err := row.Scan(&j.ID, &j.Queue, &j.Type, &j.Payload, &j.Priority, &j.Status,
		&j.Attempts, &j.MaxAttempts, &j.AvailableAt, &j.LeasedUntil, &j.LeasedBy,
		&j.LastError, &j.IdempotencyKey, &j.CreatedAt, &j.UpdatedAt)
	return j, err
}

func scanJobs(rows pgx.Rows) ([]Job, error) {
	var out []Job
	for rows.Next() {
		j, err := scanJobRow(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, j)
	}
	return out, rows.Err()
}
