// Package proxy implements the Edge Networking & Proxy Manager layer.
// It owns Caddy integration, dynamic routing, domain verification, SSL
// automation, traffic switching, preview environments, and live streaming.
package proxy

import (
	"github.com/agnivo/agnivo/packages/application/scheduler"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS proxy_routes (
	id                UUID PRIMARY KEY,
	org_id            UUID NOT NULL,
	project_id        UUID NOT NULL,
	deployment_id     UUID NOT NULL,
	domain_id         UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	hostname          TEXT NOT NULL,
	upstream          TEXT NOT NULL,
	kind              TEXT NOT NULL DEFAULT 'platform',
	status            TEXT NOT NULL DEFAULT 'pending',
	traffic_mode      TEXT NOT NULL DEFAULT 'active',
	canary_weight     INT NOT NULL DEFAULT 0,
	previous_upstream TEXT NOT NULL DEFAULT '',
	tls_enabled       BOOLEAN NOT NULL DEFAULT true,
	https_redirect    BOOLEAN NOT NULL DEFAULT true,
	strip_prefix      TEXT NOT NULL DEFAULT '',
	add_headers       JSONB NOT NULL DEFAULT '{}'::jsonb,
	timeout_seconds   INT NOT NULL DEFAULT 30,
	max_retries       INT NOT NULL DEFAULT 3,
	version           INT NOT NULL DEFAULT 1,
	correlation_id    TEXT NOT NULL DEFAULT '',
	metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS proxy_routes_hostname_uniq
	ON proxy_routes (hostname) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS proxy_routes_deployment_idx
	ON proxy_routes (deployment_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS proxy_routes_status_idx
	ON proxy_routes (status) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS proxy_routes_org_idx
	ON proxy_routes (org_id) WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS proxy_certificates (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL,
	project_id     UUID NOT NULL,
	domain_id      UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	hostname       TEXT NOT NULL,
	status         TEXT NOT NULL DEFAULT 'pending',
	issuer         TEXT NOT NULL DEFAULT 'lets_encrypt',
	serial_number  TEXT NOT NULL DEFAULT '',
	fingerprint    TEXT NOT NULL DEFAULT '',
	issued_at      TIMESTAMPTZ,
	expires_at     TIMESTAMPTZ,
	renew_after    TIMESTAMPTZ,
	is_wildcard    BOOLEAN NOT NULL DEFAULT false,
	acme_challenge TEXT NOT NULL DEFAULT '',
	failure_reason TEXT NOT NULL DEFAULT '',
	attempts       INT NOT NULL DEFAULT 0,
	correlation_id TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS proxy_certificates_hostname_uniq
	ON proxy_certificates (hostname);

CREATE INDEX IF NOT EXISTS proxy_certificates_status_idx
	ON proxy_certificates (status);

CREATE INDEX IF NOT EXISTS proxy_certificates_renew_after_idx
	ON proxy_certificates (renew_after) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS proxy_domain_verifications (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	project_id      UUID NOT NULL,
	domain_id       UUID NOT NULL,
	hostname        TEXT NOT NULL,
	method          TEXT NOT NULL DEFAULT 'txt',
	challenge_value TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'pending',
	attempts        INT NOT NULL DEFAULT 0,
	last_attempt_at TIMESTAMPTZ,
	verified_at     TIMESTAMPTZ,
	expires_at      TIMESTAMPTZ,
	failure_reason  TEXT NOT NULL DEFAULT '',
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS proxy_verifications_domain_uniq
	ON proxy_domain_verifications (domain_id);

CREATE INDEX IF NOT EXISTS proxy_verifications_status_idx
	ON proxy_domain_verifications (status);

CREATE TABLE IF NOT EXISTS proxy_previews (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL,
	project_id     UUID NOT NULL,
	deployment_id  UUID NOT NULL,
	hostname       TEXT NOT NULL,
	upstream       TEXT NOT NULL,
	branch         TEXT NOT NULL DEFAULT '',
	commit_sha     TEXT NOT NULL DEFAULT '',
	status         TEXT NOT NULL DEFAULT 'active',
	expires_at     TIMESTAMPTZ,
	correlation_id TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at     TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS proxy_previews_deployment_uniq
	ON proxy_previews (deployment_id) WHERE deleted_at IS NULL;

CREATE INDEX IF NOT EXISTS proxy_previews_expires_idx
	ON proxy_previews (expires_at) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS proxy_route_versions (
	id             UUID PRIMARY KEY,
	route_id       UUID NOT NULL,
	org_id         UUID NOT NULL,
	project_id     UUID NOT NULL,
	deployment_id  UUID NOT NULL,
	hostname       TEXT NOT NULL,
	upstream       TEXT NOT NULL,
	version        INT NOT NULL DEFAULT 1,
	correlation_id TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS proxy_route_versions_route_idx
	ON proxy_route_versions (route_id, version DESC);

CREATE TABLE IF NOT EXISTS proxy_events (
	id             UUID PRIMARY KEY,
	event_type     TEXT NOT NULL,
	org_id         UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	project_id     UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	deployment_id  UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	domain_id      UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	route_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	correlation_id TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS proxy_events_type_idx
	ON proxy_events (event_type, created_at DESC);

CREATE INDEX IF NOT EXISTS proxy_events_deployment_idx
	ON proxy_events (deployment_id, created_at DESC);
`

// Migrations returns proxy schema migrations chained after scheduler.
func Migrations() []postgres.Migration {
	m := scheduler.Migrations()
	m = append(m, postgres.Migration{Version: "0008_proxy", SQL: SchemaDDL})
	return m
}
