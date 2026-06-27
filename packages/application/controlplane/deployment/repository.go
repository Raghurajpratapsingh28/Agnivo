package deployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists deployments.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a deployment repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Create inserts a deployment and its initial timeline event.
func (r *Repository) Create(ctx context.Context, d Deployment) (Deployment, error) {
	meta, _ := json.Marshal(map[string]any{})
	if len(d.Metadata) == 0 {
		d.Metadata = meta
	}
	if d.ID == "" {
		d.ID = idx.NewUUID()
	}
	if d.Status == "" {
		d.Status = StatusPending
	}
	if d.Environment == "" {
		d.Environment = "production"
	}
	now := time.Now().UTC()
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `INSERT INTO controlplane_deployments
			(id, org_id, project_id, status, commit_sha, commit_message, branch, author,
			image_tag, runtime, trigger_source, trigger_user_id, environment, correlation_id, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$16)`
		_, err := r.db.Conn(ctx).Exec(ctx, q, d.ID, d.OrgID, d.ProjectID, d.Status,
			d.CommitSHA, d.CommitMessage, d.Branch, d.Author, d.ImageTag, d.Runtime,
			d.TriggerSource, d.TriggerUserID, d.Environment, d.CorrelationID, d.Metadata, now)
		if err != nil {
			return postgres.Translate(err, "deployment: create")
		}
		return r.addEvent(ctx, d.ID, d.Status, "deployment created", nil)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, d.OrgID, d.ID)
}

// GetByID fetches a deployment scoped to org.
func (r *Repository) GetByID(ctx context.Context, orgID, id string) (Deployment, error) {
	const q = `SELECT id, org_id, project_id, status, commit_sha, commit_message, branch, author,
		image_tag, runtime, build_duration_ms, deploy_duration_ms, failure_reason, trigger_source,
		trigger_user_id, environment, correlation_id, metadata, created_at, updated_at, started_at, finished_at
		FROM controlplane_deployments WHERE id=$1 AND org_id=$2 LIMIT 1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, id, orgID)
	d, err := scanDeployment(row)
	if err != nil {
		return Deployment{}, postgres.Translate(err, "deployment: get")
	}
	return d, nil
}

// GetLatest returns the most recent deployment for a project.
func (r *Repository) GetLatest(ctx context.Context, orgID, projectID string) (Deployment, error) {
	const q = `SELECT id, org_id, project_id, status, commit_sha, commit_message, branch, author,
		image_tag, runtime, build_duration_ms, deploy_duration_ms, failure_reason, trigger_source,
		trigger_user_id, environment, correlation_id, metadata, created_at, updated_at, started_at, finished_at
		FROM controlplane_deployments WHERE org_id=$1 AND project_id=$2
		ORDER BY created_at DESC LIMIT 1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, orgID, projectID)
	return scanDeployment(row)
}

// ListByProject returns deployment history.
func (r *Repository) ListByProject(ctx context.Context, orgID, projectID string, limit int) ([]Deployment, error) {
	if limit <= 0 {
		limit = 50
	}
	const q = `SELECT id, org_id, project_id, status, commit_sha, commit_message, branch, author,
		image_tag, runtime, build_duration_ms, deploy_duration_ms, failure_reason, trigger_source,
		trigger_user_id, environment, correlation_id, metadata, created_at, updated_at, started_at, finished_at
		FROM controlplane_deployments WHERE org_id=$1 AND project_id=$2
		ORDER BY created_at DESC LIMIT $3`
	rows, err := r.db.Conn(ctx).Query(ctx, q, orgID, projectID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "deployment: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Deployment])
}

// UpdateStatus transitions deployment status and records timeline event.
func (r *Repository) UpdateStatus(ctx context.Context, orgID, id string, status Status, message string, failureReason string) (Deployment, error) {
	now := time.Now().UTC()
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status=$3, updated_at=$4,
			failure_reason=CASE WHEN $5 <> '' THEN $5 ELSE failure_reason END,
			started_at=CASE WHEN $3 IN ('queued','building') THEN COALESCE(started_at, $4) ELSE started_at END,
			finished_at=CASE WHEN $3 IN ('live','failed','cancelled','rolled_back') THEN $4 ELSE finished_at END
			WHERE id=$1 AND org_id=$2`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, status, now, failureReason)
		if err != nil {
			return postgres.Translate(err, "deployment: update status")
		}
		if tag.RowsAffected() == 0 {
			return errors.NotFound("deployment not found")
		}
		return r.addEvent(ctx, id, status, message, nil)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, orgID, id)
}

// ListEvents returns deployment timeline.
func (r *Repository) ListEvents(ctx context.Context, deploymentID string) ([]Event, error) {
	const q = `SELECT id, deployment_id, status, message, metadata, created_at
		FROM controlplane_deployment_events WHERE deployment_id=$1 ORDER BY created_at`
	rows, err := r.db.Conn(ctx).Query(ctx, q, deploymentID)
	if err != nil {
		return nil, postgres.Translate(err, "deployment: list events")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[Event])
}

func (r *Repository) addEvent(ctx context.Context, deploymentID string, status Status, message string, meta json.RawMessage) error {
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO controlplane_deployment_events (id, deployment_id, status, message, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,now())`, idx.NewUUID(), deploymentID, status, message, meta)
	return postgres.Translate(err, "deployment: add event")
}

func scanDeployment(row pgx.Row) (Deployment, error) {
	var d Deployment
	err := row.Scan(&d.ID, &d.OrgID, &d.ProjectID, &d.Status, &d.CommitSHA, &d.CommitMessage,
		&d.Branch, &d.Author, &d.ImageTag, &d.Runtime, &d.BuildDurationMs, &d.DeployDurationMs,
		&d.FailureReason, &d.TriggerSource, &d.TriggerUserID, &d.Environment, &d.CorrelationID,
		&d.Metadata, &d.CreatedAt, &d.UpdatedAt, &d.StartedAt, &d.FinishedAt)
	return d, err
}
