// Package identity is the AgentCloud Identity & Access Platform: users,
// authentication, organizations, RBAC, API keys, sessions, multi-tenancy, and
// audit logging. Every other backend module depends on it for security.
package identity

import "github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"

// SchemaDDL is the complete identity schema, applied idempotently via
// postgres.DB.Migrate. Table names are prefixed with identity_ to avoid
// collisions with future domain tables.
const SchemaDDL = `
CREATE TABLE IF NOT EXISTS identity_users (
	id                UUID PRIMARY KEY,
	email             TEXT NOT NULL,
	email_verified_at TIMESTAMPTZ,
	password_hash     TEXT NOT NULL,
	display_name      TEXT NOT NULL DEFAULT '',
	avatar_url        TEXT NOT NULL DEFAULT '',
	timezone          TEXT NOT NULL DEFAULT 'UTC',
	locale            TEXT NOT NULL DEFAULT 'en',
	preferences       JSONB NOT NULL DEFAULT '{}'::jsonb,
	status            TEXT NOT NULL DEFAULT 'pending',
	metadata          JSONB NOT NULL DEFAULT '{}'::jsonb,
	last_login_at     TIMESTAMPTZ,
	last_active_at    TIMESTAMPTZ,
	created_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at        TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at        TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_users_email_live_uniq
	ON identity_users (lower(email))
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS identity_email_tokens (
	id         UUID PRIMARY KEY,
	user_id    UUID NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at    TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS identity_email_tokens_user_idx ON identity_email_tokens (user_id);

CREATE TABLE IF NOT EXISTS identity_password_reset_tokens (
	id         UUID PRIMARY KEY,
	user_id    UUID NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
	token_hash TEXT NOT NULL,
	expires_at TIMESTAMPTZ NOT NULL,
	used_at    TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS identity_organizations (
	id               UUID PRIMARY KEY,
	name             TEXT NOT NULL,
	slug             TEXT NOT NULL,
	avatar_url       TEXT NOT NULL DEFAULT '',
	plan_tier        TEXT NOT NULL DEFAULT 'free',
	billing_owner_id UUID REFERENCES identity_users(id),
	settings         JSONB NOT NULL DEFAULT '{}'::jsonb,
	metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
	limits           JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
	deleted_at       TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_orgs_slug_live_uniq
	ON identity_organizations (slug)
	WHERE deleted_at IS NULL;

CREATE TABLE IF NOT EXISTS identity_members (
	id         UUID PRIMARY KEY,
	org_id     UUID NOT NULL REFERENCES identity_organizations(id) ON DELETE CASCADE,
	user_id    UUID NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
	role       TEXT NOT NULL,
	status     TEXT NOT NULL DEFAULT 'active',
	invited_by UUID REFERENCES identity_users(id),
	invited_at TIMESTAMPTZ,
	joined_at  TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (org_id, user_id)
);

CREATE INDEX IF NOT EXISTS identity_members_org_idx ON identity_members (org_id) WHERE status = 'active';
CREATE INDEX IF NOT EXISTS identity_members_user_idx ON identity_members (user_id) WHERE status = 'active';

CREATE TABLE IF NOT EXISTS identity_invitations (
	id         UUID PRIMARY KEY,
	org_id     UUID NOT NULL REFERENCES identity_organizations(id) ON DELETE CASCADE,
	email      TEXT NOT NULL,
	role       TEXT NOT NULL,
	token_hash TEXT NOT NULL,
	invited_by UUID NOT NULL REFERENCES identity_users(id),
	expires_at TIMESTAMPTZ NOT NULL,
	accepted_at TIMESTAMPTZ,
	rejected_at TIMESTAMPTZ,
	created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS identity_invitations_org_email_idx
	ON identity_invitations (org_id, lower(email))
	WHERE accepted_at IS NULL AND rejected_at IS NULL;

CREATE TABLE IF NOT EXISTS identity_sessions (
	id                 UUID PRIMARY KEY,
	user_id            UUID NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
	org_id             UUID REFERENCES identity_organizations(id),
	refresh_token_hash TEXT NOT NULL,
	device_name        TEXT NOT NULL DEFAULT '',
	device_type        TEXT NOT NULL DEFAULT 'browser',
	ip_address         TEXT NOT NULL DEFAULT '',
	user_agent         TEXT NOT NULL DEFAULT '',
	remember_me        BOOLEAN NOT NULL DEFAULT false,
	expires_at         TIMESTAMPTZ NOT NULL,
	revoked_at         TIMESTAMPTZ,
	last_used_at       TIMESTAMPTZ,
	created_at         TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_sessions_refresh_hash_uniq
	ON identity_sessions (refresh_token_hash)
	WHERE revoked_at IS NULL;

CREATE INDEX IF NOT EXISTS identity_sessions_user_idx ON identity_sessions (user_id) WHERE revoked_at IS NULL;

CREATE TABLE IF NOT EXISTS identity_api_keys (
	id           UUID PRIMARY KEY,
	org_id       UUID NOT NULL REFERENCES identity_organizations(id) ON DELETE CASCADE,
	name         TEXT NOT NULL,
	prefix       TEXT NOT NULL,
	key_hash     TEXT NOT NULL,
	scopes       TEXT[] NOT NULL DEFAULT '{}',
	expires_at   TIMESTAMPTZ,
	disabled_at  TIMESTAMPTZ,
	last_used_at TIMESTAMPTZ,
	last_used_ip TEXT NOT NULL DEFAULT '',
	created_by   UUID NOT NULL REFERENCES identity_users(id),
	created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS identity_api_keys_org_idx ON identity_api_keys (org_id);
CREATE UNIQUE INDEX IF NOT EXISTS identity_api_keys_prefix_uniq ON identity_api_keys (prefix);

CREATE TABLE IF NOT EXISTS identity_personal_access_tokens (
	id           UUID PRIMARY KEY,
	user_id      UUID NOT NULL REFERENCES identity_users(id) ON DELETE CASCADE,
	org_id       UUID REFERENCES identity_organizations(id),
	name         TEXT NOT NULL,
	prefix       TEXT NOT NULL,
	token_hash   TEXT NOT NULL,
	scopes       TEXT[] NOT NULL DEFAULT '{}',
	expires_at   TIMESTAMPTZ,
	revoked_at   TIMESTAMPTZ,
	last_used_at TIMESTAMPTZ,
	metadata     JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS identity_pats_prefix_uniq ON identity_personal_access_tokens (prefix);

CREATE TABLE IF NOT EXISTS identity_audit_logs (
	id             UUID PRIMARY KEY,
	org_id         UUID,
	user_id        UUID,
	action         TEXT NOT NULL,
	resource_type  TEXT NOT NULL DEFAULT '',
	resource_id    TEXT NOT NULL DEFAULT '',
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	ip_address     TEXT NOT NULL DEFAULT '',
	user_agent     TEXT NOT NULL DEFAULT '',
	request_id     TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS identity_audit_org_idx ON identity_audit_logs (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS identity_audit_user_idx ON identity_audit_logs (user_id, created_at DESC);
`

// Migrations returns the schema as a postgres migration slice.
func Migrations() []postgres.Migration {
	return []postgres.Migration{{Version: "0001_identity", SQL: SchemaDDL}}
}
