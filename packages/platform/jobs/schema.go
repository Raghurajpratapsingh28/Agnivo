// Package jobs implements a durable, PostgreSQL-backed job queue using SELECT
// ... FOR UPDATE SKIP LOCKED for contention-free concurrent dequeue. It powers
// the builder, deployer, worker, proxy-manager, and cron executables.
//
// Features: enqueue with priorities, delayed and scheduled jobs, leases with
// heartbeats and visibility timeouts, automatic retry with exponential backoff,
// a dead-letter state, idempotency keys, failure tracking, Prometheus metrics,
// worker pools, and graceful shutdown. No external broker is required — the
// database is the single source of truth.
package jobs

// Status enumerates the lifecycle states of a job.
type Status string

const (
	// StatusPending is queued and eligible to run once available_at passes.
	StatusPending Status = "pending"
	// StatusRunning is leased by a worker; reclaimed if the lease expires.
	StatusRunning Status = "running"
	// StatusSucceeded completed successfully (retained briefly for auditing).
	StatusSucceeded Status = "succeeded"
	// StatusDead exhausted its attempts and was dead-lettered.
	StatusDead Status = "dead"
)

// Schema is the DDL for the jobs table and its supporting indexes. It is
// idempotent and intended to be applied via postgres.DB.Migrate. The partial
// indexes keep the hot dequeue path fast even with millions of completed rows.
const Schema = `
CREATE TABLE IF NOT EXISTS jobs (
	id              UUID PRIMARY KEY,
	queue           TEXT NOT NULL,
	type            TEXT NOT NULL,
	payload         JSONB NOT NULL DEFAULT '{}'::jsonb,
	priority        INT NOT NULL DEFAULT 0,
	status          TEXT NOT NULL DEFAULT 'pending',
	attempts        INT NOT NULL DEFAULT 0,
	max_attempts    INT NOT NULL DEFAULT 25,
	available_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
	leased_until    TIMESTAMPTZ,
	leased_by       TEXT,
	last_error      TEXT,
	idempotency_key TEXT,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Dedupe enqueues that carry an idempotency key, per queue.
CREATE UNIQUE INDEX IF NOT EXISTS jobs_idempotency_key_uniq
	ON jobs (queue, idempotency_key)
	WHERE idempotency_key IS NOT NULL;

-- Hot path: claim the highest-priority, longest-waiting eligible job.
CREATE INDEX IF NOT EXISTS jobs_claim_idx
	ON jobs (queue, priority DESC, available_at)
	WHERE status = 'pending';

-- Reclaim path: find running jobs whose lease has expired.
CREATE INDEX IF NOT EXISTS jobs_lease_idx
	ON jobs (leased_until)
	WHERE status = 'running';

-- GC path: purge old dead-letter jobs efficiently.
CREATE INDEX IF NOT EXISTS jobs_dead_idx
	ON jobs (updated_at)
	WHERE status = 'dead';
`

// Migration returns the schema as a slice suitable for postgres.DB.Migrate,
// versioned so it integrates with the migration bookkeeping table.
func Migration() string { return Schema }
