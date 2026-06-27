package jobs

import (
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Job is a unit of durable, retryable work.
type Job struct {
	ID             string          `json:"id"`
	Queue          string          `json:"queue"`
	Type           string          `json:"type"`
	Payload        json.RawMessage `json:"payload"`
	Priority       int             `json:"priority"`
	Status         Status          `json:"status"`
	Attempts       int             `json:"attempts"`
	MaxAttempts    int             `json:"max_attempts"`
	AvailableAt    time.Time       `json:"available_at"`
	LeasedUntil    *time.Time      `json:"leased_until,omitempty"`
	LeasedBy       *string         `json:"leased_by,omitempty"`
	LastError      *string         `json:"last_error,omitempty"`
	IdempotencyKey *string         `json:"idempotency_key,omitempty"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// Decode unmarshals the job payload into dst.
func (j Job) Decode(dst any) error {
	if len(j.Payload) == 0 {
		return errors.New(errors.CodeInvalidArgument, "jobs: empty payload")
	}
	if err := json.Unmarshal(j.Payload, dst); err != nil {
		return errors.Wrap(err, errors.CodeInvalidArgument, "jobs: decode payload")
	}
	return nil
}

// EnqueueOptions configures a single enqueue.
type EnqueueOptions struct {
	// Priority orders dequeue: higher runs first. Defaults to 0.
	Priority int
	// Delay defers availability by a relative duration (delayed jobs).
	Delay time.Duration
	// RunAt sets an absolute availability time (scheduled jobs). Takes
	// precedence over Delay when non-zero.
	RunAt time.Time
	// MaxAttempts caps retries before dead-lettering. Defaults to 25.
	MaxAttempts int
	// IdempotencyKey, when set, makes the enqueue a no-op if a job with the same
	// (queue, key) already exists, enabling safe at-least-once producers.
	IdempotencyKey string
}

func (o EnqueueOptions) maxAttempts() int {
	if o.MaxAttempts <= 0 {
		return 25
	}
	return o.MaxAttempts
}

func (o EnqueueOptions) availableAt(now time.Time) time.Time {
	if !o.RunAt.IsZero() {
		return o.RunAt.UTC()
	}
	if o.Delay > 0 {
		return now.Add(o.Delay)
	}
	return now
}
