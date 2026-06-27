package deployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// DeployResult captures successful deployment output.
type DeployResult struct {
	ImageTag         string
	ImageDigest      string
	DeployDurationMs int64
	Strategy         string
	ContainerID      string
	Metadata         json.RawMessage
}

// UpdateDeployComplete marks a deployment live with deploy metadata.
func (r *Repository) UpdateDeployComplete(ctx context.Context, orgID, id string, res DeployResult) (Deployment, error) {
	now := time.Now().UTC()
	meta := res.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status='live', image_tag=COALESCE(NULLIF($3,''), image_tag),
			deploy_duration_ms=$4, metadata=metadata || $5::jsonb,
			finished_at=$6, updated_at=$6
			WHERE id=$1 AND org_id=$2 AND status NOT IN ('cancelled','failed')`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, res.ImageTag, res.DeployDurationMs, meta, now)
		if err != nil {
			return postgres.Translate(err, "deployment: update deploy complete")
		}
		if tag.RowsAffected() == 0 {
			return errors.NotFound("deployment not found or not deployable")
		}
		evMeta, _ := json.Marshal(map[string]any{
			"strategy": res.Strategy, "container_id": res.ContainerID, "image_digest": res.ImageDigest,
		})
		return r.addEvent(ctx, id, StatusLive, "deployment live", evMeta)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, orgID, id)
}

// MarkDeployFailed transitions deployment to failed during deploy phase.
func (r *Repository) MarkDeployFailed(ctx context.Context, orgID, id, reason string, deployDurationMs int64) (Deployment, error) {
	now := time.Now().UTC()
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status='failed', failure_reason=$3,
			deploy_duration_ms=COALESCE($4, deploy_duration_ms),
			finished_at=$5, updated_at=$5
			WHERE id=$1 AND org_id=$2 AND status NOT IN ('cancelled','live')`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, reason, deployDurationMs, now)
		if err != nil {
			return postgres.Translate(err, "deployment: mark deploy failed")
		}
		if tag.RowsAffected() == 0 {
			return errors.NotFound("deployment not found")
		}
		return r.addEvent(ctx, id, StatusFailed, reason, nil)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, orgID, id)
}

// MarkRolledBack completes a rollback deployment.
func (r *Repository) MarkRolledBack(ctx context.Context, orgID, id string, deployDurationMs int64) (Deployment, error) {
	now := time.Now().UTC()
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status='rolled_back', deploy_duration_ms=$3, finished_at=$4, updated_at=$4
			WHERE id=$1 AND org_id=$2`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, deployDurationMs, now)
		if err != nil {
			return postgres.Translate(err, "deployment: mark rolled back")
		}
		if tag.RowsAffected() == 0 {
			return errors.NotFound("deployment not found")
		}
		return r.addEvent(ctx, id, StatusRolledBack, "rollback completed", nil)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, orgID, id)
}

// GetPreviousLive returns the most recent live deployment for a project before excludeID.
func (r *Repository) GetPreviousLive(ctx context.Context, orgID, projectID, excludeID string) (Deployment, error) {
	const q = `SELECT id, org_id, project_id, status, commit_sha, commit_message, branch, author,
		image_tag, runtime, build_duration_ms, deploy_duration_ms, failure_reason, trigger_source,
		trigger_user_id, environment, correlation_id, metadata, created_at, updated_at, started_at, finished_at
		FROM controlplane_deployments
		WHERE org_id=$1 AND project_id=$2 AND status='live' AND id <> $3
		ORDER BY finished_at DESC NULLS LAST, created_at DESC LIMIT 1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, orgID, projectID, excludeID)
	return scanDeployment(row)
}

// IsReadyToDeploy reports whether the deployment has a built image ready.
func (r *Repository) IsReadyToDeploy(ctx context.Context, orgID, id string) (bool, error) {
	const q = `SELECT status, image_tag FROM controlplane_deployments WHERE id=$1 AND org_id=$2`
	var status Status
	var tag string
	err := r.db.Conn(ctx).QueryRow(ctx, q, id, orgID).Scan(&status, &tag)
	if err != nil {
		return false, postgres.Translate(err, "deployment: ready check")
	}
	if status == StatusRollingBack && tag != "" {
		return true, nil
	}
	return status == StatusBuilt && tag != "", nil
}
