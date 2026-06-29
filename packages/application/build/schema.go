// Package build implements the Builder worker: git checkout, framework detection,
// Dockerfile generation, BuildKit execution, ECR push, and artifact persistence.
package build

import (
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS builder_builds (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	deployment_id   UUID NOT NULL,
	job_id          UUID,
	status          TEXT NOT NULL DEFAULT 'queued',
	framework       TEXT NOT NULL DEFAULT '',
	runtime         TEXT NOT NULL DEFAULT '',
	commit_sha      TEXT NOT NULL DEFAULT '',
	branch          TEXT NOT NULL DEFAULT '',
	environment     TEXT NOT NULL DEFAULT 'production',
	correlation_id  TEXT NOT NULL DEFAULT '',
	worker_id       TEXT NOT NULL DEFAULT '',
	started_at      TIMESTAMPTZ,
	finished_at     TIMESTAMPTZ,
	failure_reason  TEXT NOT NULL DEFAULT '',
	cancelled_at    TIMESTAMPTZ,
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS builder_builds_deployment_uniq
	ON builder_builds (deployment_id);

CREATE INDEX IF NOT EXISTS builder_builds_org_project_idx
	ON builder_builds (org_id, project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS builder_builds_status_idx
	ON builder_builds (status) WHERE status IN ('queued','running');

CREATE TABLE IF NOT EXISTS builder_build_logs (
	id              UUID PRIMARY KEY,
	build_id        UUID NOT NULL REFERENCES builder_builds(id) ON DELETE CASCADE,
	deployment_id   UUID NOT NULL,
	stage           TEXT NOT NULL DEFAULT '',
	level           TEXT NOT NULL DEFAULT 'info',
	message         TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS builder_build_logs_build_idx
	ON builder_build_logs (build_id, created_at);

CREATE INDEX IF NOT EXISTS builder_build_logs_deployment_idx
	ON builder_build_logs (deployment_id, created_at);

CREATE TABLE IF NOT EXISTS builder_artifacts (
	id                  UUID PRIMARY KEY,
	build_id            UUID NOT NULL REFERENCES builder_builds(id) ON DELETE CASCADE,
	deployment_id       UUID NOT NULL,
	org_id              UUID NOT NULL,
	project_id          UUID NOT NULL,
	image_digest        TEXT NOT NULL DEFAULT '',
	image_tag           TEXT NOT NULL DEFAULT '',
	image_size_bytes    BIGINT NOT NULL DEFAULT 0,
	registry            TEXT NOT NULL DEFAULT '',
	repository          TEXT NOT NULL DEFAULT '',
	framework           TEXT NOT NULL DEFAULT '',
	runtime             TEXT NOT NULL DEFAULT '',
	dockerfile_version  TEXT NOT NULL DEFAULT '',
	builder_version     TEXT NOT NULL DEFAULT '',
	commit_sha          TEXT NOT NULL DEFAULT '',
	branch              TEXT NOT NULL DEFAULT '',
	build_duration_ms   BIGINT NOT NULL DEFAULT 0,
	clone_duration_ms   BIGINT NOT NULL DEFAULT 0,
	docker_duration_ms  BIGINT NOT NULL DEFAULT 0,
	push_duration_ms    BIGINT NOT NULL DEFAULT 0,
	cache_hit_ratio     DOUBLE PRECISION NOT NULL DEFAULT 0,
	cache_layers_hit    INT NOT NULL DEFAULT 0,
	cache_layers_total  INT NOT NULL DEFAULT 0,
	warnings            JSONB NOT NULL DEFAULT '[]'::jsonb,
	sbom                JSONB,
	metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS builder_artifacts_deployment_uniq
	ON builder_artifacts (deployment_id);

CREATE TABLE IF NOT EXISTS builder_events (
	id              UUID PRIMARY KEY,
	build_id        UUID NOT NULL,
	deployment_id   UUID NOT NULL,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	event_type      TEXT NOT NULL,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS builder_events_build_idx
	ON builder_events (build_id, created_at);

CREATE INDEX IF NOT EXISTS builder_events_deployment_idx
	ON builder_events (deployment_id, created_at);
`

// Migrations returns builder schema migrations including controlplane prerequisites.
func Migrations() []postgres.Migration {
	m := controlplane.Migrations()
	m = append(m, postgres.Migration{Version: "0004_builder", SQL: SchemaDDL})
	return m
}
