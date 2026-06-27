// Package controlplane is the Project Management & Deployment Control Plane.
// It orchestrates projects, repositories, deployments, env vars, secrets, and
// domains — enqueueing jobs and publishing events for builder/deployer workers.
package controlplane

import (
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/jobs"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS controlplane_projects (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	name            TEXT NOT NULL,
	slug            TEXT NOT NULL,
	description     TEXT NOT NULL DEFAULT '',
	repo_url        TEXT NOT NULL DEFAULT '',
	repo_provider   TEXT NOT NULL DEFAULT '',
	branch          TEXT NOT NULL DEFAULT 'main',
	default_runtime TEXT NOT NULL DEFAULT '',
	framework       TEXT NOT NULL DEFAULT '',
	build_method    TEXT NOT NULL DEFAULT 'dockerfile',
	region          TEXT NOT NULL DEFAULT 'us-east-1',
	sleep_config    JSONB NOT NULL DEFAULT '{}'::jsonb,
	visibility      TEXT NOT NULL DEFAULT 'private',
	status          TEXT NOT NULL DEFAULT 'active',
	labels          JSONB NOT NULL DEFAULT '[]'::jsonb,
	tags            TEXT[] NOT NULL DEFAULT '{}',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_by      UUID NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	archived_at     TIMESTAMPTZ,
	deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_projects_org_slug_uniq
	ON controlplane_projects (org_id, slug) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS controlplane_projects_org_idx
	ON controlplane_projects (org_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS controlplane_git_repositories (
	id              UUID PRIMARY KEY,
	project_id      UUID NOT NULL REFERENCES controlplane_projects(id) ON DELETE CASCADE,
	org_id          UUID NOT NULL,
	provider        TEXT NOT NULL,
	repo_url        TEXT NOT NULL,
	clone_url       TEXT NOT NULL DEFAULT '',
	default_branch  TEXT NOT NULL DEFAULT 'main',
	is_private      BOOLEAN NOT NULL DEFAULT false,
	access_token_enc BYTEA,
	deploy_key_enc  BYTEA,
	webhook_secret  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	connected_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	disconnected_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_git_repos_project_uniq
	ON controlplane_git_repositories (project_id) WHERE disconnected_at IS NULL;

CREATE TABLE IF NOT EXISTS controlplane_webhook_deliveries (
	id              UUID PRIMARY KEY,
	provider        TEXT NOT NULL,
	delivery_id     TEXT NOT NULL,
	event_type      TEXT NOT NULL,
	project_id      UUID,
	payload_hash    TEXT NOT NULL,
	received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
	processed_at    TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_webhook_delivery_uniq
	ON controlplane_webhook_deliveries (provider, delivery_id);

CREATE TABLE IF NOT EXISTS controlplane_deployments (
	id                  UUID PRIMARY KEY,
	org_id              UUID NOT NULL,
	project_id          UUID NOT NULL REFERENCES controlplane_projects(id),
	status              TEXT NOT NULL DEFAULT 'pending',
	commit_sha          TEXT NOT NULL DEFAULT '',
	commit_message      TEXT NOT NULL DEFAULT '',
	branch              TEXT NOT NULL DEFAULT '',
	author              TEXT NOT NULL DEFAULT '',
	image_tag           TEXT NOT NULL DEFAULT '',
	runtime             TEXT NOT NULL DEFAULT '',
	build_duration_ms   BIGINT,
	deploy_duration_ms  BIGINT,
	failure_reason      TEXT NOT NULL DEFAULT '',
	trigger_source      TEXT NOT NULL DEFAULT 'manual',
	trigger_user_id     UUID,
	environment         TEXT NOT NULL DEFAULT 'production',
	correlation_id      TEXT NOT NULL DEFAULT '',
	metadata            JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
	started_at          TIMESTAMPTZ,
	finished_at         TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS controlplane_deployments_project_idx
	ON controlplane_deployments (project_id, created_at DESC);

CREATE INDEX IF NOT EXISTS controlplane_deployments_org_idx
	ON controlplane_deployments (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS controlplane_deployment_events (
	id              UUID PRIMARY KEY,
	deployment_id   UUID NOT NULL REFERENCES controlplane_deployments(id) ON DELETE CASCADE,
	status          TEXT NOT NULL,
	message         TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS controlplane_deployment_events_dep_idx
	ON controlplane_deployment_events (deployment_id, created_at);

CREATE TABLE IF NOT EXISTS controlplane_env_vars (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL REFERENCES controlplane_projects(id) ON DELETE CASCADE,
	key             TEXT NOT NULL,
	value_enc       BYTEA NOT NULL,
	environment     TEXT NOT NULL DEFAULT 'production',
	is_secret       BOOLEAN NOT NULL DEFAULT false,
	version         INT NOT NULL DEFAULT 1,
	created_by      UUID NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_env_vars_key_uniq
	ON controlplane_env_vars (project_id, environment, key) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS controlplane_env_var_versions (
	id              UUID PRIMARY KEY,
	env_var_id      UUID NOT NULL REFERENCES controlplane_env_vars(id) ON DELETE CASCADE,
	version         INT NOT NULL,
	value_enc       BYTEA NOT NULL,
	changed_by      UUID NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS controlplane_secrets (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL REFERENCES controlplane_projects(id) ON DELETE CASCADE,
	name            TEXT NOT NULL,
	value_enc       BYTEA NOT NULL,
	environment     TEXT NOT NULL DEFAULT 'production',
	version         INT NOT NULL DEFAULT 1,
	disabled_at     TIMESTAMPTZ,
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_by      UUID NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_secrets_name_uniq
	ON controlplane_secrets (project_id, environment, name) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS controlplane_secret_versions (
	id              UUID PRIMARY KEY,
	secret_id       UUID NOT NULL REFERENCES controlplane_secrets(id) ON DELETE CASCADE,
	version         INT NOT NULL,
	value_enc       BYTEA NOT NULL,
	rotated_by      UUID NOT NULL,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS controlplane_domains (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL REFERENCES controlplane_projects(id) ON DELETE CASCADE,
	hostname        TEXT NOT NULL,
	domain_type     TEXT NOT NULL DEFAULT 'custom',
	is_primary      BOOLEAN NOT NULL DEFAULT false,
	is_preview      BOOLEAN NOT NULL DEFAULT false,
	verification_status TEXT NOT NULL DEFAULT 'pending',
	ssl_status      TEXT NOT NULL DEFAULT 'pending',
	redirect_to     TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at      TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS controlplane_domains_hostname_uniq
	ON controlplane_domains (hostname) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS controlplane_domains_project_idx
	ON controlplane_domains (project_id) WHERE deleted_at IS NULL;
`

// Migrations returns controlplane and jobs schema migrations.
func Migrations() []postgres.Migration {
	return []postgres.Migration{
		{Version: "0002_controlplane", SQL: SchemaDDL},
		{Version: "0003_jobs", SQL: jobs.Migration()},
	}
}
