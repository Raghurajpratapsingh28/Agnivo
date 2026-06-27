package deploystore

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists deployer executions.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an execution repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// UpsertForDeployment idempotently creates an execution row.
func (r *Repository) UpsertForDeployment(ctx context.Context, e model.Execution) (model.Execution, error) {
	if e.ID == "" {
		e.ID = idx.NewUUID()
	}
	meta := e.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO deployer_executions (id, org_id, project_id, deployment_id, job_id, status, phase, strategy,
	environment, image_tag, image_digest, build_id, correlation_id, worker_id, rollback_of, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,now(),now())
ON CONFLICT (deployment_id) DO UPDATE SET job_id=COALESCE(EXCLUDED.job_id, deployer_executions.job_id), updated_at=now()
RETURNING id, org_id, project_id, deployment_id, job_id, status, phase, strategy, environment, image_tag,
	image_digest, build_id, container_id, host, port, correlation_id, worker_id, failure_reason, rollback_of,
	started_at, finished_at, deploy_duration_ms, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q, e.ID, e.OrgID, e.ProjectID, e.DeploymentID, e.JobID,
		e.Status, e.Phase, e.Strategy, e.Environment, e.ImageTag, e.ImageDigest, e.BuildID,
		e.CorrelationID, e.WorkerID, e.RollbackOf, meta)
	return scanExecution(row)
}

// SetPhase transitions execution phase and records transition.
func (r *Repository) SetPhase(ctx context.Context, execID, deploymentID string, from, to model.Phase, message string) error {
	return r.db.Transact(ctx, func(ctx context.Context) error {
		_, err := r.db.Conn(ctx).Exec(ctx,
			`UPDATE deployer_executions SET phase=$2, updated_at=now() WHERE id=$1`, execID, to)
		if err != nil {
			return postgres.Translate(err, "deployer: set phase")
		}
		meta, _ := json.Marshal(map[string]any{})
		_, err = r.db.Conn(ctx).Exec(ctx,
			`INSERT INTO deployer_phase_transitions (id, execution_id, deployment_id, from_phase, to_phase, message, metadata, created_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
			idx.NewUUID(), execID, deploymentID, string(from), string(to), message, meta)
		return postgres.Translate(err, "deployer: phase transition")
	})
}

// MarkRunning starts an execution.
func (r *Repository) MarkRunning(ctx context.Context, id, workerID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_executions SET status='running', worker_id=$2, started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1`,
		id, workerID)
	return postgres.Translate(err, "deployer: mark running")
}

// MarkSucceeded completes execution successfully.
func (r *Repository) MarkSucceeded(ctx context.Context, id, containerID, host string, port int, durationMs int64) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_executions SET status='succeeded', phase='deployment_complete', container_id=$2,
		host=$3, port=$4, deploy_duration_ms=$5, finished_at=now(), updated_at=now() WHERE id=$1`,
		id, containerID, host, port, durationMs)
	return postgres.Translate(err, "deployer: mark succeeded")
}

// MarkFailed records execution failure.
func (r *Repository) MarkFailed(ctx context.Context, id, reason string, durationMs int64) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_executions SET status='failed', phase='deployment_failed', failure_reason=$2,
		deploy_duration_ms=$3, finished_at=now(), updated_at=now() WHERE id=$1`,
		id, reason, durationMs)
	return postgres.Translate(err, "deployer: mark failed")
}

// MarkCancelled records cancellation.
func (r *Repository) MarkCancelled(ctx context.Context, id string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_executions SET status='cancelled', phase='cancelled', finished_at=now(), updated_at=now() WHERE id=$1`, id)
	return postgres.Translate(err, "deployer: mark cancelled")
}

// GetByDeployment returns execution for a deployment.
func (r *Repository) GetByDeployment(ctx context.Context, deploymentID string) (model.Execution, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, job_id, status, phase, strategy, environment,
		image_tag, image_digest, build_id, container_id, host, port, correlation_id, worker_id, failure_reason,
		rollback_of, started_at, finished_at, deploy_duration_ms, metadata, created_at, updated_at
		FROM deployer_executions WHERE deployment_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, deploymentID)
	return scanExecution(row)
}

// Stats returns execution counts by status.
func (r *Repository) Stats(ctx context.Context) (map[model.ExecutionStatus]int64, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT status, count(*) FROM deployer_executions GROUP BY status`)
	if err != nil {
		return nil, postgres.Translate(err, "deployer: stats")
	}
	defer rows.Close()
	out := make(map[model.ExecutionStatus]int64)
	for rows.Next() {
		var s model.ExecutionStatus
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

// ListPhaseTransitions returns phase history for a deployment.
func (r *Repository) ListPhaseTransitions(ctx context.Context, deploymentID string) ([]model.PhaseTransition, error) {
	const q = `SELECT id, execution_id, deployment_id, from_phase, to_phase, message, metadata, created_at
		FROM deployer_phase_transitions WHERE deployment_id=$1 ORDER BY created_at`
	rows, err := r.db.Conn(ctx).Query(ctx, q, deploymentID)
	if err != nil {
		return nil, postgres.Translate(err, "deployer: list phases")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.PhaseTransition])
}

func scanExecution(row pgx.Row) (model.Execution, error) {
	var e model.Execution
	err := row.Scan(&e.ID, &e.OrgID, &e.ProjectID, &e.DeploymentID, &e.JobID, &e.Status, &e.Phase,
		&e.Strategy, &e.Environment, &e.ImageTag, &e.ImageDigest, &e.BuildID, &e.ContainerID,
		&e.Host, &e.Port, &e.CorrelationID, &e.WorkerID, &e.FailureReason, &e.RollbackOf,
		&e.StartedAt, &e.FinishedAt, &e.DeployDurationMs, &e.Metadata, &e.CreatedAt, &e.UpdatedAt)
	if err != nil {
		return model.Execution{}, postgres.Translate(err, "deployer: scan execution")
	}
	return e, nil
}

// ContainerRepository tracks container lifecycle.
type ContainerRepository struct{ db *postgres.DB }

// NewContainerRepository constructs a container repository.
func NewContainerRepository(db *postgres.DB) *ContainerRepository {
	return &ContainerRepository{db: db}
}

// Create inserts a container record.
func (r *ContainerRepository) Create(ctx context.Context, c model.ContainerRecord) error {
	if c.ID == "" {
		c.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO deployer_containers (id, execution_id, deployment_id, container_id, image, host, port, status, role, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`,
		c.ID, c.ExecutionID, c.DeploymentID, c.ContainerID, c.Image, c.Host, c.Port, c.Status, c.Role)
	return postgres.Translate(err, "deployer: create container")
}

// MarkStopped updates container stopped time.
func (r *ContainerRepository) MarkStopped(ctx context.Context, containerID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_containers SET status='stopped', stopped_at=now() WHERE container_id=$1`, containerID)
	return postgres.Translate(err, "deployer: stop container")
}

// MarkRemoved updates container removed time.
func (r *ContainerRepository) MarkRemoved(ctx context.Context, containerID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE deployer_containers SET status='removed', removed_at=now() WHERE container_id=$1`, containerID)
	return postgres.Translate(err, "deployer: remove container")
}

// ListActiveByProject returns non-removed containers for cleanup.
func (r *ContainerRepository) ListActiveByDeployment(ctx context.Context, deploymentID string) ([]model.ContainerRecord, error) {
	const q = `SELECT id, execution_id, deployment_id, container_id, image, host, port, status, role, created_at, stopped_at, removed_at
		FROM deployer_containers WHERE deployment_id=$1 AND removed_at IS NULL ORDER BY created_at`
	rows, err := r.db.Conn(ctx).Query(ctx, q, deploymentID)
	if err != nil {
		return nil, postgres.Translate(err, "deployer: list containers")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.ContainerRecord])
}

// ListActiveByProject returns active containers for a project (for drain/cleanup).
func (r *ContainerRepository) ListActiveByProject(ctx context.Context, orgID, projectID string) ([]model.ContainerRecord, error) {
	const q = `SELECT c.id, c.execution_id, c.deployment_id, c.container_id, c.image, c.host, c.port, c.status, c.role, c.created_at, c.stopped_at, c.removed_at
		FROM deployer_containers c
		JOIN deployer_executions e ON e.id = c.execution_id
		WHERE e.org_id=$1 AND e.project_id=$2 AND c.removed_at IS NULL AND c.role='active'
		ORDER BY c.created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID)
	if err != nil {
		return nil, postgres.Translate(err, "deployer: list project containers")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.ContainerRecord])
}

// HealthRepository persists health check results.
type HealthRepository struct{ db *postgres.DB }

// NewHealthRepository constructs a health repository.
func NewHealthRepository(db *postgres.DB) *HealthRepository { return &HealthRepository{db: db} }

// Record inserts a health check result.
func (r *HealthRepository) Record(ctx context.Context, h model.HealthRecord) error {
	if h.ID == "" {
		h.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO deployer_health_checks (id, execution_id, deployment_id, check_type, success, latency_ms, message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
		h.ID, h.ExecutionID, h.DeploymentID, h.CheckType, h.Success, h.LatencyMs, h.Message)
	return postgres.Translate(err, "deployer: record health")
}

// ListByDeployment returns health history.
func (r *HealthRepository) ListByDeployment(ctx context.Context, deploymentID string, limit int) ([]model.HealthRecord, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT id, execution_id, deployment_id, check_type, success, latency_ms, message, created_at
		FROM deployer_health_checks WHERE deployment_id=$1 ORDER BY created_at DESC LIMIT $2`
	rows, err := r.db.Conn(ctx).Query(ctx, q, deploymentID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "deployer: list health")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.HealthRecord])
}
