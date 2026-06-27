// Package runtimeagent implements the node runtime execution engine.
package runtimeagent

import (
	"github.com/agnivo/agnivo/packages/application/scheduler"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS runtime_containers (
	id              UUID PRIMARY KEY,
	container_id    TEXT NOT NULL UNIQUE,
	deployment_id   TEXT NOT NULL DEFAULT '',
	name            TEXT NOT NULL DEFAULT '',
	image           TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'created',
	host_port       INT NOT NULL DEFAULT 0,
	container_port  INT NOT NULL DEFAULT 8080,
	restart_count   INT NOT NULL DEFAULT 0,
	oom_killed      BOOLEAN NOT NULL DEFAULT false,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	started_at      TIMESTAMPTZ,
	stopped_at      TIMESTAMPTZ,
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS runtime_containers_deployment_idx ON runtime_containers (deployment_id);
CREATE INDEX IF NOT EXISTS runtime_containers_status_idx ON runtime_containers (status);

CREATE TABLE IF NOT EXISTS runtime_container_transitions (
	id              UUID PRIMARY KEY,
	container_id    TEXT NOT NULL,
	from_status     TEXT NOT NULL DEFAULT '',
	to_status       TEXT NOT NULL,
	message         TEXT NOT NULL DEFAULT '',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runtime_health_checks (
	id              UUID PRIMARY KEY,
	container_id    TEXT NOT NULL,
	check_type      TEXT NOT NULL,
	success         BOOLEAN NOT NULL,
	cpu_percent     DOUBLE PRECISION NOT NULL DEFAULT 0,
	memory_mb       BIGINT NOT NULL DEFAULT 0,
	message         TEXT NOT NULL DEFAULT '',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runtime_events (
	id              UUID PRIMARY KEY,
	event_type      TEXT NOT NULL,
	container_id    TEXT NOT NULL DEFAULT '',
	deployment_id   TEXT NOT NULL DEFAULT '',
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS runtime_logs (
	id              UUID PRIMARY KEY,
	container_id    TEXT NOT NULL,
	stream          TEXT NOT NULL DEFAULT 'stdout',
	line            TEXT NOT NULL,
	correlation_id  TEXT NOT NULL DEFAULT '',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS runtime_logs_container_idx ON runtime_logs (container_id, created_at DESC);
`

// Migrations returns runtime agent schema migrations chained after scheduler.
func Migrations() []postgres.Migration {
	m := scheduler.Migrations()
	m = append(m, postgres.Migration{Version: "0007_runtimeagent", SQL: SchemaDDL})
	return m
}
