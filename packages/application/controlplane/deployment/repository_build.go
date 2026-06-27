package deployment

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// BuildResult captures the output of a successful build.
type BuildResult struct {
	ImageTag        string
	ImageDigest     string
	Runtime         string
	Framework       string
	BuildDurationMs int64
	Metadata        json.RawMessage
}

// UpdateBuildComplete records build success: image tag, digest, duration, and status built.
func (r *Repository) UpdateBuildComplete(ctx context.Context, orgID, id string, res BuildResult) (Deployment, error) {
	now := time.Now().UTC()
	meta := res.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status='built', image_tag=$3, runtime=$4, build_duration_ms=$5,
			metadata = metadata || $6::jsonb, updated_at=$7
			WHERE id=$1 AND org_id=$2 AND status NOT IN ('cancelled','failed')`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, res.ImageTag, res.Runtime, res.BuildDurationMs, meta, now)
		if err != nil {
			return postgres.Translate(err, "deployment: update build complete")
		}
		if tag.RowsAffected() == 0 {
			return errors.NotFound("deployment not found or not buildable")
		}
		metaMap := map[string]any{"image_digest": res.ImageDigest, "framework": res.Framework}
		evMeta, _ := json.Marshal(metaMap)
		return r.addEvent(ctx, id, StatusBuilt, "build completed", evMeta)
	})
	if err != nil {
		return Deployment{}, err
	}
	return r.GetByID(ctx, orgID, id)
}

// MarkBuildFailed transitions a deployment to failed with a reason.
func (r *Repository) MarkBuildFailed(ctx context.Context, orgID, id, reason string, buildDurationMs int64) (Deployment, error) {
	now := time.Now().UTC()
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `UPDATE controlplane_deployments SET
			status='failed', failure_reason=$3, build_duration_ms=COALESCE($4, build_duration_ms),
			finished_at=$5, updated_at=$5
			WHERE id=$1 AND org_id=$2 AND status NOT IN ('cancelled','live')`
		tag, err := r.db.Conn(ctx).Exec(ctx, q, id, orgID, reason, buildDurationMs, now)
		if err != nil {
			return postgres.Translate(err, "deployment: mark build failed")
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

// IsCancelled reports whether the deployment was cancelled.
func (r *Repository) IsCancelled(ctx context.Context, orgID, id string) (bool, error) {
	const q = `SELECT status FROM controlplane_deployments WHERE id=$1 AND org_id=$2`
	var status Status
	err := r.db.Conn(ctx).QueryRow(ctx, q, id, orgID).Scan(&status)
	if err != nil {
		return false, postgres.Translate(err, "deployment: is cancelled")
	}
	return status == StatusCancelled, nil
}
