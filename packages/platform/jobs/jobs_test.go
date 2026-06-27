package jobs_test

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestJobDecode(t *testing.T) {
	j := jobs.Job{Payload: []byte(`{"name":"web"}`)}
	var p struct {
		Name string `json:"name"`
	}
	require.NoError(t, j.Decode(&p))
	assert.Equal(t, "web", p.Name)

	require.Error(t, jobs.Job{}.Decode(&p)) // empty payload
}

func TestMetricsNilSafe(t *testing.T) {
	var m *jobs.Metrics
	assert.Nil(t, m.Collectors())
	assert.NotEmpty(t, jobs.NewMetrics("svc").Collectors())
}

func TestSchemaConstant(t *testing.T) {
	assert.Contains(t, jobs.Schema, "CREATE TABLE IF NOT EXISTS jobs")
	assert.Contains(t, jobs.Schema, "jobs_claim_idx")
	assert.Equal(t, jobs.Schema, jobs.Migration())
}

// --- Integration tests (gated on DATABASE_TEST_URL) ---

func dialDB(t *testing.T) *postgres.DB {
	t.Helper()
	url := os.Getenv("DATABASE_TEST_URL")
	if url == "" {
		t.Skip("DATABASE_TEST_URL not set; skipping jobs integration test")
	}
	cfg := &config.Config{Database: config.Database{Enabled: true, URL: url, MaxConns: 5, QueryTimeout: 5 * time.Second}}
	db, err := postgres.New(context.Background(), cfg)
	require.NoError(t, err)
	t.Cleanup(db.Close)
	require.NoError(t, db.Migrate(context.Background(), []postgres.Migration{{Version: "0001_jobs", SQL: jobs.Schema}}))
	return db
}

func TestEnqueueDequeueCompleteIntegration(t *testing.T) {
	db := dialDB(t)
	ctx := context.Background()
	q := jobs.NewQueue(db, nil)
	queueName := "test_q"

	// Clean slate for this queue.
	_, _ = db.Exec(ctx, "DELETE FROM jobs WHERE queue=$1", queueName)

	job, err := q.Enqueue(ctx, queueName, "send_email", map[string]string{"to": "a@b.c"}, jobs.EnqueueOptions{Priority: 5})
	require.NoError(t, err)
	assert.Equal(t, jobs.StatusPending, job.Status)

	claimed, err := q.Dequeue(ctx, queueName, "w1", 10, 30*time.Second)
	require.NoError(t, err)
	require.Len(t, claimed, 1)
	assert.Equal(t, jobs.StatusRunning, claimed[0].Status)
	assert.Equal(t, 1, claimed[0].Attempts)

	// A second worker sees nothing (SKIP LOCKED + lease).
	none, err := q.Dequeue(ctx, queueName, "w2", 10, 30*time.Second)
	require.NoError(t, err)
	assert.Empty(t, none)

	require.NoError(t, q.Complete(ctx, claimed[0].ID))
}

func TestIdempotentEnqueueIntegration(t *testing.T) {
	db := dialDB(t)
	ctx := context.Background()
	q := jobs.NewQueue(db, nil)
	_, _ = db.Exec(ctx, "DELETE FROM jobs WHERE queue=$1", "idem_q")

	opts := jobs.EnqueueOptions{IdempotencyKey: "order-42"}
	a, err := q.Enqueue(ctx, "idem_q", "process", nil, opts)
	require.NoError(t, err)
	b, err := q.Enqueue(ctx, "idem_q", "process", nil, opts)
	require.NoError(t, err)
	assert.Equal(t, a.ID, b.ID) // deduped to the same job
}

func TestFailRetryThenDeadIntegration(t *testing.T) {
	db := dialDB(t)
	ctx := context.Background()
	q := jobs.NewQueue(db, nil)
	_, _ = db.Exec(ctx, "DELETE FROM jobs WHERE queue=$1", "fail_q")

	job, err := q.Enqueue(ctx, "fail_q", "x", nil, jobs.EnqueueOptions{MaxAttempts: 1})
	require.NoError(t, err)

	claimed, err := q.Dequeue(ctx, "fail_q", "w1", 1, time.Second)
	require.NoError(t, err)
	require.Len(t, claimed, 1)

	status, err := q.Fail(ctx, job.ID, assertErr("boom"), 0)
	require.NoError(t, err)
	assert.Equal(t, jobs.StatusDead, status) // exhausted max_attempts=1 after one claim
}

type assertErr string

func (e assertErr) Error() string { return string(e) }
