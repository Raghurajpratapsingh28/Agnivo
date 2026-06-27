package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository persists scheduler state.
type Repository struct{ db *postgres.DB }

// NewRepository constructs a scheduler repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// UpsertServer registers or updates a server from heartbeat.
func (r *Repository) UpsertServer(ctx context.Context, hb model.HeartbeatPayload) (model.Server, error) {
	labels, _ := json.Marshal(hb.Labels)
	meta, _ := json.Marshal(hb.Metadata)
	if labels == nil {
		labels = []byte("{}")
	}
	if meta == nil {
		meta = []byte("{}")
	}
	const q = `
INSERT INTO scheduler_servers (id, node_id, hostname, advertise_host, agent_url, region, availability_zone,
	instance_type, architecture, os, kernel_version, docker_version, cpu_cores, memory_mb, disk_gb, gpu_count,
	max_containers, container_count, health_status, labels, metadata, last_heartbeat, missed_beats, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,$19,$20,$21,now(),0,now())
ON CONFLICT (node_id) DO UPDATE SET
	hostname=EXCLUDED.hostname, advertise_host=EXCLUDED.advertise_host, agent_url=EXCLUDED.agent_url,
	region=EXCLUDED.region, availability_zone=EXCLUDED.availability_zone, instance_type=EXCLUDED.instance_type,
	architecture=EXCLUDED.architecture, os=EXCLUDED.os, kernel_version=EXCLUDED.kernel_version,
	docker_version=EXCLUDED.docker_version, cpu_cores=EXCLUDED.cpu_cores, memory_mb=EXCLUDED.memory_mb,
	disk_gb=EXCLUDED.disk_gb, gpu_count=EXCLUDED.gpu_count, max_containers=EXCLUDED.max_containers,
	container_count=EXCLUDED.container_count, health_status=EXCLUDED.health_status,
	labels=EXCLUDED.labels, metadata=EXCLUDED.metadata, last_heartbeat=now(), missed_beats=0, updated_at=now()
RETURNING id, node_id, hostname, advertise_host, agent_url, region, availability_zone, instance_type,
	architecture, os, kernel_version, docker_version, cpu_cores, memory_mb, disk_gb, gpu_count,
	max_containers, container_count, reserved_cpu, reserved_memory_mb, reserved_disk_gb, health_status,
	labels, metadata, last_heartbeat, missed_beats, created_at, updated_at`
	id := idx.NewUUID()
	row := r.db.Conn(ctx).QueryRow(ctx, q, id, hb.NodeID, hb.Hostname, hb.AdvertiseHost, hb.AgentURL,
		hb.Region, hb.AvailabilityZone, hb.InstanceType, hb.Architecture, hb.OS, hb.KernelVersion,
		hb.DockerVersion, hb.CPUCores, hb.MemoryMB, hb.DiskGB, hb.GPUCount, hb.MaxContainers,
		hb.ContainerCount, hb.HealthStatus, labels, meta)
	return scanServer(row)
}

// RemoveServer marks a server offline and removes from active pool.
func (r *Repository) RemoveServer(ctx context.Context, nodeID string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE scheduler_servers SET health_status='offline', updated_at=now() WHERE node_id=$1`, nodeID)
	return postgres.Translate(err, "scheduler: remove server")
}

// ListHealthyServers returns online servers optionally filtered by region.
func (r *Repository) ListHealthyServers(ctx context.Context, region string) ([]model.Server, error) {
	q := `SELECT id, node_id, hostname, advertise_host, agent_url, region, availability_zone, instance_type,
		architecture, os, kernel_version, docker_version, cpu_cores, memory_mb, disk_gb, gpu_count,
		max_containers, container_count, reserved_cpu, reserved_memory_mb, reserved_disk_gb, health_status,
		labels, metadata, last_heartbeat, missed_beats, created_at, updated_at
		FROM scheduler_servers WHERE health_status IN ('healthy','degraded')`
	args := []any{}
	if region != "" {
		q += ` AND region=$1`
		args = append(args, region)
	}
	q += ` ORDER BY container_count ASC, reserved_cpu ASC`
	rows, err := r.db.Conn(ctx).Query(ctx, q, args...)
	if err != nil {
		return nil, postgres.Translate(err, "scheduler: list servers")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Server])
}

// GetServerByNodeID returns a server by node ID.
func (r *Repository) GetServerByNodeID(ctx context.Context, nodeID string) (model.Server, error) {
	const q = `SELECT id, node_id, hostname, advertise_host, agent_url, region, availability_zone, instance_type,
		architecture, os, kernel_version, docker_version, cpu_cores, memory_mb, disk_gb, gpu_count,
		max_containers, container_count, reserved_cpu, reserved_memory_mb, reserved_disk_gb, health_status,
		labels, metadata, last_heartbeat, missed_beats, created_at, updated_at
		FROM scheduler_servers WHERE node_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, nodeID)
	return scanServer(row)
}

// CreateReservation persists a new reservation and updates server accounting.
func (r *Repository) CreateReservation(ctx context.Context, res model.Reservation) (model.Reservation, error) {
	meta := res.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	if res.ID == "" {
		res.ID = idx.NewUUID()
	}
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		const q = `INSERT INTO scheduler_reservations (id, org_id, project_id, deployment_id, server_id, node_id,
			host, port, cpu_millicores, memory_mb, algorithm, status, expires_at, correlation_id, metadata, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,now(),now())
			ON CONFLICT (deployment_id) DO UPDATE SET server_id=EXCLUDED.server_id, host=EXCLUDED.host, port=EXCLUDED.port,
			status='active', expires_at=EXCLUDED.expires_at, updated_at=now()
			RETURNING id`
		if err := r.db.Conn(ctx).QueryRow(ctx, q, res.ID, res.OrgID, res.ProjectID, res.DeploymentID,
			res.ServerID, res.NodeID, res.Host, res.Port, res.CPUMillicores, res.MemoryMB, res.Algorithm,
			res.Status, res.ExpiresAt, res.CorrelationID, meta).Scan(&res.ID); err != nil {
			return postgres.Translate(err, "scheduler: create reservation")
		}
		cpu := float64(res.CPUMillicores) / 1000.0
		_, err := r.db.Conn(ctx).Exec(ctx,
			`UPDATE scheduler_servers SET reserved_cpu=reserved_cpu+$2, reserved_memory_mb=reserved_memory_mb+$3,
			container_count=container_count+1, updated_at=now() WHERE id=$1`, res.ServerID, cpu, int64(res.MemoryMB))
		return postgres.Translate(err, "scheduler: reserve resources")
	})
	if err != nil {
		return model.Reservation{}, err
	}
	return res, nil
}

// ReleaseReservation releases resources for a deployment.
func (r *Repository) ReleaseReservation(ctx context.Context, orgID, projectID, deploymentID string) error {
	return r.db.Transact(ctx, func(ctx context.Context) error {
		const sel = `SELECT server_id, cpu_millicores, memory_mb, status FROM scheduler_reservations
			WHERE deployment_id=$1 AND org_id=$2 AND project_id=$3 AND status='active'`
		var serverID string
		var cpuMilli, mem int
		var status string
		err := r.db.Conn(ctx).QueryRow(ctx, sel, deploymentID, orgID, projectID).Scan(&serverID, &cpuMilli, &mem, &status)
		if err != nil {
			if postgres.IsNotFound(err) {
				return nil
			}
			return postgres.Translate(err, "scheduler: find reservation")
		}
		_, err = r.db.Conn(ctx).Exec(ctx,
			`UPDATE scheduler_reservations SET status='released', updated_at=now()
			WHERE deployment_id=$1 AND status='active'`, deploymentID)
		if err != nil {
			return postgres.Translate(err, "scheduler: release reservation")
		}
		cpu := float64(cpuMilli) / 1000.0
		_, err = r.db.Conn(ctx).Exec(ctx,
			`UPDATE scheduler_servers SET reserved_cpu=GREATEST(0, reserved_cpu-$2),
			reserved_memory_mb=GREATEST(0, reserved_memory_mb-$3),
			container_count=GREATEST(0, container_count-1), updated_at=now() WHERE id=$1`,
			serverID, cpu, int64(mem))
		return postgres.Translate(err, "scheduler: release resources")
	})
}

// GetReservation returns active reservation for deployment.
func (r *Repository) GetReservation(ctx context.Context, deploymentID string) (model.Reservation, error) {
	const q = `SELECT id, org_id, project_id, deployment_id, server_id, node_id, host, port, cpu_millicores,
		memory_mb, algorithm, status, expires_at, correlation_id, metadata, created_at, updated_at
		FROM scheduler_reservations WHERE deployment_id=$1 AND status='active'`
	row := r.db.Conn(ctx).QueryRow(ctx, q, deploymentID)
	var res model.Reservation
	err := row.Scan(&res.ID, &res.OrgID, &res.ProjectID, &res.DeploymentID, &res.ServerID, &res.NodeID,
		&res.Host, &res.Port, &res.CPUMillicores, &res.MemoryMB, &res.Algorithm, &res.Status,
		&res.ExpiresAt, &res.CorrelationID, &res.Metadata, &res.CreatedAt, &res.UpdatedAt)
	if err != nil {
		return model.Reservation{}, postgres.Translate(err, "scheduler: get reservation")
	}
	return res, nil
}

// ExpireStaleReservations marks expired reservations and frees resources.
func (r *Repository) ExpireStaleReservations(ctx context.Context) (int64, error) {
	var count int64
	err := r.db.Transact(ctx, func(ctx context.Context) error {
		rows, err := r.db.Conn(ctx).Query(ctx,
			`SELECT id, server_id, cpu_millicores, memory_mb FROM scheduler_reservations
			WHERE status='active' AND expires_at < now()`)
		if err != nil {
			return postgres.Translate(err, "scheduler: list expired")
		}
		defer rows.Close()
		for rows.Next() {
			var id, serverID string
			var cpuMilli, mem int
			if err := rows.Scan(&id, &serverID, &cpuMilli, &mem); err != nil {
				return err
			}
			_, err = r.db.Conn(ctx).Exec(ctx,
				`UPDATE scheduler_reservations SET status='expired', updated_at=now() WHERE id=$1`, id)
			if err != nil {
				return err
			}
			cpu := float64(cpuMilli) / 1000.0
			_, err = r.db.Conn(ctx).Exec(ctx,
				`UPDATE scheduler_servers SET reserved_cpu=GREATEST(0, reserved_cpu-$2),
				reserved_memory_mb=GREATEST(0, reserved_memory_mb-$3),
				container_count=GREATEST(0, container_count-1), updated_at=now() WHERE id=$1`,
				serverID, cpu, int64(mem))
			if err != nil {
				return err
			}
			count++
		}
		return rows.Err()
	})
	return count, err
}

// MarkMissedHeartbeats increments missed beats and marks offline servers.
func (r *Repository) MarkMissedHeartbeats(ctx context.Context, threshold time.Duration, maxMissed int) (int64, error) {
	res, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE scheduler_servers SET missed_beats=missed_beats+1, updated_at=now()
WHERE health_status IN ('healthy','degraded') AND (last_heartbeat IS NULL OR last_heartbeat < now() - $1 * interval '1 second')`, int(threshold.Seconds()))
	if err != nil {
		return 0, postgres.Translate(err, "scheduler: mark missed")
	}
	res2, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE scheduler_servers SET health_status='offline', updated_at=now()
		WHERE missed_beats >= $1 AND health_status != 'offline'`, maxMissed)
	if err != nil {
		return 0, postgres.Translate(err, "scheduler: mark offline")
	}
	return res.RowsAffected() + res2.RowsAffected(), nil
}

// CapacitySummary returns aggregate capacity stats.
func (r *Repository) CapacitySummary(ctx context.Context) (map[string]any, error) {
	const q = `SELECT count(*) FILTER (WHERE health_status='healthy') AS healthy,
		count(*) FILTER (WHERE health_status='offline') AS offline,
		COALESCE(sum(cpu_cores),0) AS total_cpu, COALESCE(sum(memory_mb),0) AS total_memory,
		COALESCE(sum(reserved_cpu),0) AS reserved_cpu, COALESCE(sum(reserved_memory_mb),0) AS reserved_memory,
		COALESCE(sum(container_count),0) AS containers
		FROM scheduler_servers`
	row := r.db.Conn(ctx).QueryRow(ctx, q)
	var healthy, offline int
	var totalCPU, reservedCPU float64
	var totalMem, reservedMem, containers int64
	if err := row.Scan(&healthy, &offline, &totalCPU, &totalMem, &reservedCPU, &reservedMem, &containers); err != nil {
		return nil, postgres.Translate(err, "scheduler: capacity")
	}
	return map[string]any{
		"healthy_servers": healthy, "offline_servers": offline,
		"total_cpu_cores": totalCPU, "reserved_cpu_cores": reservedCPU,
		"total_memory_mb": totalMem, "reserved_memory_mb": reservedMem,
		"container_count": containers,
	}, nil
}

func scanServer(row pgx.Row) (model.Server, error) {
	var s model.Server
	err := row.Scan(&s.ID, &s.NodeID, &s.Hostname, &s.AdvertiseHost, &s.AgentURL, &s.Region,
		&s.AvailabilityZone, &s.InstanceType, &s.Architecture, &s.OS, &s.KernelVersion, &s.DockerVersion,
		&s.CPUCores, &s.MemoryMB, &s.DiskGB, &s.GPUCount, &s.MaxContainers, &s.ContainerCount,
		&s.ReservedCPU, &s.ReservedMemoryMB, &s.ReservedDiskGB, &s.HealthStatus,
		&s.Labels, &s.Metadata, &s.LastHeartbeat, &s.MissedBeats, &s.CreatedAt, &s.UpdatedAt)
	if err != nil {
		return model.Server{}, postgres.Translate(err, "scheduler: scan server")
	}
	return s, nil
}
