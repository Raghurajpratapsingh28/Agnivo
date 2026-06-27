// Package scheduler implements the placement scheduler service.
package scheduler

import (
	"github.com/agnivo/agnivo/packages/application/deploy"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS scheduler_servers (
	id              UUID PRIMARY KEY,
	node_id         TEXT NOT NULL UNIQUE,
	hostname        TEXT NOT NULL DEFAULT '',
	advertise_host  TEXT NOT NULL DEFAULT '',
	agent_url       TEXT NOT NULL DEFAULT '',
	region          TEXT NOT NULL DEFAULT '',
	availability_zone TEXT NOT NULL DEFAULT '',
	instance_type   TEXT NOT NULL DEFAULT '',
	architecture    TEXT NOT NULL DEFAULT 'amd64',
	os              TEXT NOT NULL DEFAULT '',
	kernel_version  TEXT NOT NULL DEFAULT '',
	docker_version  TEXT NOT NULL DEFAULT '',
	cpu_cores       DOUBLE PRECISION NOT NULL DEFAULT 0,
	memory_mb       BIGINT NOT NULL DEFAULT 0,
	disk_gb         BIGINT NOT NULL DEFAULT 0,
	gpu_count       INT NOT NULL DEFAULT 0,
	max_containers  INT NOT NULL DEFAULT 0,
	container_count INT NOT NULL DEFAULT 0,
	reserved_cpu    DOUBLE PRECISION NOT NULL DEFAULT 0,
	reserved_memory_mb BIGINT NOT NULL DEFAULT 0,
	reserved_disk_gb BIGINT NOT NULL DEFAULT 0,
	health_status   TEXT NOT NULL DEFAULT 'healthy',
	labels          JSONB NOT NULL DEFAULT '{}'::jsonb,
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	last_heartbeat  TIMESTAMPTZ,
	missed_beats    INT NOT NULL DEFAULT 0,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS scheduler_servers_region_idx ON scheduler_servers (region, health_status);
CREATE INDEX IF NOT EXISTS scheduler_servers_heartbeat_idx ON scheduler_servers (last_heartbeat);

CREATE TABLE IF NOT EXISTS scheduler_reservations (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	deployment_id   UUID NOT NULL UNIQUE,
	server_id       UUID NOT NULL REFERENCES scheduler_servers(id),
	node_id         TEXT NOT NULL,
	host            TEXT NOT NULL,
	port            INT NOT NULL,
	cpu_millicores  INT NOT NULL DEFAULT 250,
	memory_mb       INT NOT NULL DEFAULT 512,
	algorithm       TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'active',
	expires_at      TIMESTAMPTZ NOT NULL,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS scheduler_reservations_server_idx ON scheduler_reservations (server_id, status);
CREATE INDEX IF NOT EXISTS scheduler_reservations_expires_idx ON scheduler_reservations (expires_at) WHERE status='active';

CREATE TABLE IF NOT EXISTS scheduler_events (
	id              UUID PRIMARY KEY,
	event_type      TEXT NOT NULL,
	org_id          UUID,
	project_id      UUID,
	deployment_id   UUID,
	server_id       UUID,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS scheduler_events_type_idx ON scheduler_events (event_type, created_at DESC);
`

// Migrations returns scheduler schema migrations chained after deployer.
func Migrations() []postgres.Migration {
	m := deploy.Migrations()
	m = append(m, postgres.Migration{Version: "0006_scheduler", SQL: SchemaDDL})
	return m
}
