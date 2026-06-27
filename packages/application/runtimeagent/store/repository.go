package store

import (
	"context"
	"encoding/json"

	"github.com/agnivo/agnivo/packages/application/runtimeagent/model"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists runtime agent state.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a runtime repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// UpsertContainer creates or updates a container record.
func (r *Repository) UpsertContainer(ctx context.Context, rec model.ContainerRecord) error {
	meta := rec.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	if rec.ID == "" {
		rec.ID = idx.NewUUID()
	}
	const q = `
INSERT INTO runtime_containers (id, container_id, deployment_id, name, image, status, host_port, container_port,
	restart_count, oom_killed, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now(),now())
ON CONFLICT (container_id) DO UPDATE SET status=$6, host_port=$7, restart_count=$9, oom_killed=$10, updated_at=now()`
	_, err := r.db.Conn(ctx).Exec(ctx, q, rec.ID, rec.ContainerID, rec.DeploymentID, rec.Name, rec.Image,
		rec.Status, rec.HostPort, rec.ContainerPort, rec.RestartCount, rec.OOMKilled, rec.CorrelationID, meta)
	return postgres.Translate(err, "runtime: upsert container")
}

// SetStatus updates container status and records transition.
func (r *Repository) SetStatus(ctx context.Context, containerID string, from, to model.ContainerStatus, message string) error {
	return r.db.Transact(ctx, func(ctx context.Context) error {
		_, err := r.db.Conn(ctx).Exec(ctx,
			`UPDATE runtime_containers SET status=$2, updated_at=now(),
			started_at=CASE WHEN $2='running' THEN COALESCE(started_at, now()) ELSE started_at END,
			stopped_at=CASE WHEN $2 IN ('stopped','deleted','failed') THEN now() ELSE stopped_at END
			WHERE container_id=$1`, containerID, to)
		if err != nil {
			return postgres.Translate(err, "runtime: set status")
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`INSERT INTO runtime_container_transitions (id, container_id, from_status, to_status, message, created_at)
			VALUES ($1,$2,$3,$4,$5,now())`, idx.NewUUID(), containerID, string(from), string(to), message)
		return postgres.Translate(err, "runtime: transition")
	})
}

// GetByContainerID returns a container record.
func (r *Repository) GetByContainerID(ctx context.Context, containerID string) (model.ContainerRecord, error) {
	const q = `SELECT id, container_id, deployment_id, name, image, status, host_port, container_port,
		restart_count, oom_killed, correlation_id, metadata, created_at, started_at, stopped_at, updated_at
		FROM runtime_containers WHERE container_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, containerID)
	var rec model.ContainerRecord
	err := row.Scan(&rec.ID, &rec.ContainerID, &rec.DeploymentID, &rec.Name, &rec.Image, &rec.Status,
		&rec.HostPort, &rec.ContainerPort, &rec.RestartCount, &rec.OOMKilled, &rec.CorrelationID,
		&rec.Metadata, &rec.CreatedAt, &rec.StartedAt, &rec.StoppedAt, &rec.UpdatedAt)
	if err != nil {
		return model.ContainerRecord{}, postgres.Translate(err, "runtime: get container")
	}
	return rec, nil
}

// ListActive returns non-deleted containers.
func (r *Repository) ListActive(ctx context.Context) ([]model.ContainerRecord, error) {
	const q = `SELECT id, container_id, deployment_id, name, image, status, host_port, container_port,
		restart_count, oom_killed, correlation_id, metadata, created_at, started_at, stopped_at, updated_at
		FROM runtime_containers WHERE status NOT IN ('deleted','failed') ORDER BY created_at DESC`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "runtime: list containers")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.ContainerRecord])
}

// RecordHealth inserts a health check result.
func (r *Repository) RecordHealth(ctx context.Context, containerID, checkType string, success bool, cpu float64, mem int64, message string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO runtime_health_checks (id, container_id, check_type, success, cpu_percent, memory_mb, message, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
		idx.NewUUID(), containerID, checkType, success, cpu, mem, message)
	return postgres.Translate(err, "runtime: record health")
}

// AppendLog stores a log line.
func (r *Repository) AppendLog(ctx context.Context, containerID, stream, line, correlationID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO runtime_logs (id, container_id, stream, line, correlation_id, created_at) VALUES ($1,$2,$3,$4,$5,now())`,
		idx.NewUUID(), containerID, stream, line, correlationID)
	return postgres.Translate(err, "runtime: append log")
}

// ListLogs returns recent log lines.
func (r *Repository) ListLogs(ctx context.Context, containerID string, limit int) ([]model.LogLine, error) {
	if limit <= 0 {
		limit = 100
	}
	const q = `SELECT container_id, stream, line, correlation_id, created_at FROM runtime_logs
		WHERE container_id=$1 ORDER BY created_at DESC LIMIT $2`
	rows, err := r.db.Conn(ctx).Query(ctx, q, containerID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "runtime: list logs")
	}
	defer rows.Close()
	var out []model.LogLine
	for rows.Next() {
		var l model.LogLine
		if err := rows.Scan(&l.ContainerID, &l.Stream, &l.Line, &l.CorrelationID, &l.Timestamp); err != nil {
			return nil, err
		}
		out = append(out, l)
	}
	return out, rows.Err()
}

// CountActive returns active container count.
func (r *Repository) CountActive(ctx context.Context) (int, error) {
	var n int
	err := r.db.Conn(ctx).QueryRow(ctx,
		`SELECT count(*) FROM runtime_containers WHERE status IN ('running','starting','restarting')`).Scan(&n)
	return n, postgres.Translate(err, "runtime: count active")
}
