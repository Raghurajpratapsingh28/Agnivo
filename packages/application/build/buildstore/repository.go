package buildstore

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists build records.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a build repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// UpsertForDeployment creates or returns the build row for a deployment (idempotent).
func (r *Repository) UpsertForDeployment(ctx context.Context, b model.Build) (model.Build, error) {
	if b.ID == "" {
		b.ID = idx.NewUUID()
	}
	meta := b.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO builder_builds (id, org_id, project_id, deployment_id, job_id, status, framework, runtime,
	commit_sha, branch, environment, correlation_id, worker_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now())
ON CONFLICT (deployment_id) DO UPDATE SET
	job_id = COALESCE(EXCLUDED.job_id, builder_builds.job_id),
	updated_at = now()
RETURNING id, org_id, project_id, deployment_id, job_id, status, framework, runtime,
	commit_sha, branch, environment, correlation_id, worker_id, started_at, finished_at,
	failure_reason, cancelled_at, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q, b.ID, b.OrgID, b.ProjectID, b.DeploymentID, b.JobID,
		b.Status, b.Framework, b.Runtime, b.CommitSHA, b.Branch, b.Environment,
		b.CorrelationID, b.WorkerID, meta)
	return scanBuild(row)
}

// MarkRunning transitions a build to running.
func (r *Repository) MarkRunning(ctx context.Context, id, workerID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE builder_builds SET status='running', worker_id=$2, started_at=COALESCE(started_at, now()), updated_at=now() WHERE id=$1`,
		id, workerID)
	return postgres.Translate(err, "build: mark running")
}

// MarkSucceeded completes a build successfully.
func (r *Repository) MarkSucceeded(ctx context.Context, id, framework, runtime string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE builder_builds SET status='succeeded', framework=$2, runtime=$3, finished_at=now(), updated_at=now() WHERE id=$1`,
		id, framework, runtime)
	return postgres.Translate(err, "build: mark succeeded")
}

// MarkFailed records build failure.
func (r *Repository) MarkFailed(ctx context.Context, id, reason string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE builder_builds SET status='failed', failure_reason=$2, finished_at=now(), updated_at=now() WHERE id=$1`,
		id, reason)
	return postgres.Translate(err, "build: mark failed")
}

// MarkCancelled records build cancellation.
func (r *Repository) MarkCancelled(ctx context.Context, id string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE builder_builds SET status='cancelled', cancelled_at=now(), finished_at=now(), updated_at=now() WHERE id=$1`,
		id)
	return postgres.Translate(err, "build: mark cancelled")
}

// GetByDeployment returns the build for a deployment.
func (r *Repository) GetByDeployment(ctx context.Context, deploymentID string) (model.Build, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, job_id, status, framework, runtime,
		commit_sha, branch, environment, correlation_id, worker_id, started_at, finished_at,
		failure_reason, cancelled_at, metadata, created_at, updated_at
		FROM builder_builds WHERE deployment_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, deploymentID)
	return scanBuild(row)
}

// GetByID returns a build by ID.
func (r *Repository) GetByID(ctx context.Context, id string) (model.Build, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, job_id, status, framework, runtime,
		commit_sha, branch, environment, correlation_id, worker_id, started_at, finished_at,
		failure_reason, cancelled_at, metadata, created_at, updated_at
		FROM builder_builds WHERE id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, id)
	return scanBuild(row)
}

// Stats returns build counts by status.
func (r *Repository) Stats(ctx context.Context) (map[model.BuildStatus]int64, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `SELECT status, count(*) FROM builder_builds GROUP BY status`)
	if err != nil {
		return nil, postgres.Translate(err, "build: stats")
	}
	defer rows.Close()
	out := make(map[model.BuildStatus]int64)
	for rows.Next() {
		var s model.BuildStatus
		var n int64
		if err := rows.Scan(&s, &n); err != nil {
			return nil, err
		}
		out[s] = n
	}
	return out, rows.Err()
}

func scanBuild(row pgx.Row) (model.Build, error) {
	var b model.Build
	err := row.Scan(&b.ID, &b.OrgID, &b.ProjectID, &b.DeploymentID, &b.JobID, &b.Status,
		&b.Framework, &b.Runtime, &b.CommitSHA, &b.Branch, &b.Environment, &b.CorrelationID,
		&b.WorkerID, &b.StartedAt, &b.FinishedAt, &b.FailureReason, &b.CancelledAt,
		&b.Metadata, &b.CreatedAt, &b.UpdatedAt)
	if err != nil {
		return model.Build{}, postgres.Translate(err, "build: scan")
	}
	return b, nil
}

// ArtifactRepository persists build artifacts.
type ArtifactRepository struct{ db *postgres.DB }

// NewArtifactRepository constructs an artifact repository.
func NewArtifactRepository(db *postgres.DB) *ArtifactRepository { return &ArtifactRepository{db: db} }

// Save inserts an artifact (idempotent on deployment_id).
func (r *ArtifactRepository) Save(ctx context.Context, a model.Artifact) (model.Artifact, error) {
	if a.ID == "" {
		a.ID = idx.NewUUID()
	}
	warnings := a.Warnings
	if warnings == nil {
		warnings, _ = json.Marshal([]string{})
	}
	meta := a.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO builder_artifacts (
	id, build_id, deployment_id, org_id, project_id, image_digest, image_tag, image_size_bytes,
	registry, repository, framework, runtime, dockerfile_version, builder_version, commit_sha, branch,
	build_duration_ms, clone_duration_ms, docker_duration_ms, push_duration_ms,
	cache_hit_ratio, cache_layers_hit, cache_layers_total, warnings, sbom, metadata, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,$22,$23,$24,$25,$26,now())
ON CONFLICT (deployment_id) DO UPDATE SET
	image_digest=EXCLUDED.image_digest, image_tag=EXCLUDED.image_tag, image_size_bytes=EXCLUDED.image_size_bytes,
	build_duration_ms=EXCLUDED.build_duration_ms, metadata=EXCLUDED.metadata, created_at=now()
RETURNING id, created_at`
	err := r.db.Conn(ctx).QueryRow(ctx, q, a.ID, a.BuildID, a.DeploymentID, a.OrgID, a.ProjectID,
		a.ImageDigest, a.ImageTag, a.ImageSizeBytes, a.Registry, a.Repository, a.Framework, a.Runtime,
		a.DockerfileVersion, a.BuilderVersion, a.CommitSHA, a.Branch, a.BuildDurationMs,
		a.CloneDurationMs, a.DockerDurationMs, a.PushDurationMs, a.CacheHitRatio,
		a.CacheLayersHit, a.CacheLayersTotal, warnings, a.SBOM, meta).Scan(&a.ID, &a.CreatedAt)
	if err != nil {
		return model.Artifact{}, postgres.Translate(err, "artifact: save")
	}
	return a, nil
}

// GetByDeployment returns the artifact for a deployment.
func (r *ArtifactRepository) GetByDeployment(ctx context.Context, deploymentID string) (model.Artifact, error) {
	const q = `SELECT id, build_id, deployment_id, org_id, project_id, image_digest, image_tag,
		image_size_bytes, registry, repository, framework, runtime, dockerfile_version, builder_version,
		commit_sha, branch, build_duration_ms, clone_duration_ms, docker_duration_ms, push_duration_ms,
		cache_hit_ratio, cache_layers_hit, cache_layers_total, warnings, sbom, metadata, created_at
		FROM builder_artifacts WHERE deployment_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, deploymentID)
	var a model.Artifact
	err := row.Scan(&a.ID, &a.BuildID, &a.DeploymentID, &a.OrgID, &a.ProjectID, &a.ImageDigest,
		&a.ImageTag, &a.ImageSizeBytes, &a.Registry, &a.Repository, &a.Framework, &a.Runtime,
		&a.DockerfileVersion, &a.BuilderVersion, &a.CommitSHA, &a.Branch, &a.BuildDurationMs,
		&a.CloneDurationMs, &a.DockerDurationMs, &a.PushDurationMs, &a.CacheHitRatio,
		&a.CacheLayersHit, &a.CacheLayersTotal, &a.Warnings, &a.SBOM, &a.Metadata, &a.CreatedAt)
	if err != nil {
		return model.Artifact{}, postgres.Translate(err, "artifact: get")
	}
	return a, nil
}

// LogRepository persists and queries build logs.
type LogRepository struct{ db *postgres.DB }

// NewLogRepository constructs a log repository.
func NewLogRepository(db *postgres.DB) *LogRepository { return &LogRepository{db: db} }

// Append inserts a log entry.
func (r *LogRepository) Append(ctx context.Context, entry model.LogEntry) error {
	meta := entry.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	if entry.ID == "" {
		entry.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO builder_build_logs (id, build_id, deployment_id, stage, level, message, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
		entry.ID, entry.BuildID, entry.DeploymentID, entry.Stage, entry.Level, entry.Message, meta)
	return postgres.Translate(err, "build logs: append")
}

// ListByBuild returns log entries for a build.
func (r *LogRepository) ListByBuild(ctx context.Context, buildID string, limit int) ([]model.LogEntry, error) {
	if limit <= 0 {
		limit = 500
	}
	const q = `SELECT id, build_id, deployment_id, stage, level, message, metadata, created_at
		FROM builder_build_logs WHERE build_id=$1 ORDER BY created_at LIMIT $2`
	rows, err := r.db.Conn(ctx).Query(ctx, q, buildID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "build logs: list")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.LogEntry])
}

// ArchiveOlderThan deletes logs older than the given duration (cleanup job).
func (r *LogRepository) ArchiveOlderThan(ctx context.Context, olderThan time.Duration) (int64, error) {
	tag, err := r.db.Conn(ctx).Exec(ctx,
		`DELETE FROM builder_build_logs WHERE created_at < now() - $1::interval`, olderThan.String())
	if err != nil {
		return 0, postgres.Translate(err, "build logs: archive")
	}
	return tag.RowsAffected(), nil
}

// NotFound is a sentinel for missing builds.
var NotFound = errors.NotFound("build not found")
