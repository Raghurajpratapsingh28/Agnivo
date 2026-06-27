// Package deploy implements the Deployer worker: image pull, runtime preparation,
// deployment strategies, health verification, rollback, and container lifecycle.
package deploy

import (
	"github.com/agnivo/agnivo/packages/application/build"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS deployer_executions (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	deployment_id   UUID NOT NULL,
	job_id          UUID,
	status          TEXT NOT NULL DEFAULT 'queued',
	phase           TEXT NOT NULL DEFAULT 'pending',
	strategy        TEXT NOT NULL DEFAULT 'rolling',
	environment     TEXT NOT NULL DEFAULT 'production',
	image_tag       TEXT NOT NULL DEFAULT '',
	image_digest    TEXT NOT NULL DEFAULT '',
	build_id        UUID,
	container_id    TEXT NOT NULL DEFAULT '',
	host            TEXT NOT NULL DEFAULT '',
	port            INT NOT NULL DEFAULT 0,
	correlation_id  TEXT NOT NULL DEFAULT '',
	worker_id       TEXT NOT NULL DEFAULT '',
	failure_reason  TEXT NOT NULL DEFAULT '',
	rollback_of     UUID,
	started_at      TIMESTAMPTZ,
	finished_at     TIMESTAMPTZ,
	deploy_duration_ms BIGINT,
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS deployer_executions_deployment_uniq
	ON deployer_executions (deployment_id);

CREATE INDEX IF NOT EXISTS deployer_executions_phase_idx
	ON deployer_executions (phase) WHERE status IN ('running','queued');

CREATE TABLE IF NOT EXISTS deployer_phase_transitions (
	id              UUID PRIMARY KEY,
	execution_id    UUID NOT NULL REFERENCES deployer_executions(id) ON DELETE CASCADE,
	deployment_id   UUID NOT NULL,
	from_phase      TEXT NOT NULL DEFAULT '',
	to_phase        TEXT NOT NULL,
	message         TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deployer_phase_transitions_dep_idx
	ON deployer_phase_transitions (deployment_id, created_at);

CREATE TABLE IF NOT EXISTS deployer_containers (
	id              UUID PRIMARY KEY,
	execution_id    UUID NOT NULL REFERENCES deployer_executions(id) ON DELETE CASCADE,
	deployment_id   UUID NOT NULL,
	container_id    TEXT NOT NULL,
	image           TEXT NOT NULL,
	host            TEXT NOT NULL DEFAULT '',
	port            INT NOT NULL DEFAULT 0,
	status          TEXT NOT NULL DEFAULT 'creating',
	role            TEXT NOT NULL DEFAULT 'active',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	stopped_at      TIMESTAMPTZ,
	removed_at      TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS deployer_containers_deployment_idx
	ON deployer_containers (deployment_id);

CREATE TABLE IF NOT EXISTS deployer_health_checks (
	id              UUID PRIMARY KEY,
	execution_id    UUID NOT NULL,
	deployment_id   UUID NOT NULL,
	check_type      TEXT NOT NULL,
	success         BOOLEAN NOT NULL,
	latency_ms      BIGINT NOT NULL DEFAULT 0,
	message         TEXT NOT NULL DEFAULT '',
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deployer_health_checks_dep_idx
	ON deployer_health_checks (deployment_id, created_at DESC);

CREATE TABLE IF NOT EXISTS deployer_events (
	id              UUID PRIMARY KEY,
	execution_id    UUID NOT NULL,
	deployment_id   UUID NOT NULL,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	event_type      TEXT NOT NULL,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS deployer_events_deployment_idx
	ON deployer_events (deployment_id, created_at);
`

// Migrations returns deployer schema migrations chained after builder.
func Migrations() []postgres.Migration {
	m := build.Migrations()
	m = append(m, postgres.Migration{Version: "0005_deployer", SQL: SchemaDDL})
	return m
}
