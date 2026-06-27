// Package ops is the Platform Operations composition root.
// It owns billing, metering, quota enforcement, notifications, backups,
// garbage collection, analytics, audit logging, and auto-sleep.
package ops

import (
	"github.com/agnivo/agnivo/packages/application/proxy"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
)

const SchemaDDL = `
CREATE TABLE IF NOT EXISTS ops_plans (
	id                  TEXT PRIMARY KEY,
	name                TEXT NOT NULL,
	price_cents_month   BIGINT NOT NULL DEFAULT 0,
	price_cents_year    BIGINT NOT NULL DEFAULT 0,
	stripe_price_id     TEXT NOT NULL DEFAULT '',
	features            JSONB NOT NULL DEFAULT '{}'::jsonb,
	active              BOOLEAN NOT NULL DEFAULT true,
	created_at          TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at          TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO ops_plans (id, name, price_cents_month, price_cents_year, active)
VALUES
	('free',       'Free',       0,      0,       true),
	('pro',        'Pro',        2000,   19200,   true),
	('team',       'Team',       7900,   75840,   true),
	('enterprise', 'Enterprise', 0,      0,       true)
ON CONFLICT (id) DO NOTHING;

CREATE TABLE IF NOT EXISTS ops_subscriptions (
	id                   UUID PRIMARY KEY,
	org_id               UUID NOT NULL UNIQUE,
	plan_id              TEXT NOT NULL DEFAULT 'free' REFERENCES ops_plans(id),
	status               TEXT NOT NULL DEFAULT 'active',
	interval             TEXT NOT NULL DEFAULT 'monthly',
	stripe_sub_id        TEXT NOT NULL DEFAULT '',
	stripe_customer_id   TEXT NOT NULL DEFAULT '',
	current_period_start TIMESTAMPTZ NOT NULL DEFAULT now(),
	current_period_end   TIMESTAMPTZ NOT NULL DEFAULT now() + INTERVAL '1 month',
	trial_ends_at        TIMESTAMPTZ,
	canceled_at          TIMESTAMPTZ,
	grace_period_ends_at TIMESTAMPTZ,
	correlation_id       TEXT NOT NULL DEFAULT '',
	metadata             JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at           TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at           TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_subscriptions_status_idx ON ops_subscriptions (status);
CREATE INDEX IF NOT EXISTS ops_subscriptions_period_end_idx ON ops_subscriptions (current_period_end);

CREATE TABLE IF NOT EXISTS ops_invoices (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	subscription_id UUID NOT NULL,
	status          TEXT NOT NULL DEFAULT 'pending',
	period_start    TIMESTAMPTZ NOT NULL,
	period_end      TIMESTAMPTZ NOT NULL,
	amount_cents    BIGINT NOT NULL DEFAULT 0,
	credits_cents   BIGINT NOT NULL DEFAULT 0,
	tax_cents       BIGINT NOT NULL DEFAULT 0,
	total_cents     BIGINT NOT NULL DEFAULT 0,
	currency        TEXT NOT NULL DEFAULT 'usd',
	stripe_inv_id   TEXT NOT NULL DEFAULT '',
	paid_at         TIMESTAMPTZ,
	due_at          TIMESTAMPTZ,
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_invoices_org_idx ON ops_invoices (org_id, created_at DESC);
CREATE INDEX IF NOT EXISTS ops_invoices_status_idx ON ops_invoices (status);

CREATE TABLE IF NOT EXISTS ops_credits (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL,
	amount_cents   BIGINT NOT NULL DEFAULT 0,
	used_cents     BIGINT NOT NULL DEFAULT 0,
	reason         TEXT NOT NULL DEFAULT '',
	coupon_code    TEXT NOT NULL DEFAULT '',
	expires_at     TIMESTAMPTZ,
	correlation_id TEXT NOT NULL DEFAULT '',
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_credits_org_idx ON ops_credits (org_id);

CREATE TABLE IF NOT EXISTS ops_quota_configs (
	id                        UUID PRIMARY KEY,
	plan_id                   TEXT NOT NULL UNIQUE REFERENCES ops_plans(id),
	max_projects              BIGINT NOT NULL DEFAULT 3,
	max_deployments           BIGINT NOT NULL DEFAULT 10,
	max_concurrent_builds     BIGINT NOT NULL DEFAULT 2,
	max_concurrent_deploys    BIGINT NOT NULL DEFAULT 2,
	max_containers            BIGINT NOT NULL DEFAULT 5,
	max_custom_domains        BIGINT NOT NULL DEFAULT 1,
	max_storage_gb            NUMERIC(10,3) NOT NULL DEFAULT 1.0,
	max_bandwidth_gb_month    NUMERIC(12,3) NOT NULL DEFAULT 100.0,
	max_build_minutes_month   NUMERIC(12,3) NOT NULL DEFAULT 500.0,
	max_container_hours_month NUMERIC(12,3) NOT NULL DEFAULT 720.0,
	max_api_requests_day      BIGINT NOT NULL DEFAULT 100000,
	warn_threshold_pct        NUMERIC(5,2) NOT NULL DEFAULT 80.0,
	created_at                TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at                TIMESTAMPTZ NOT NULL DEFAULT now()
);

INSERT INTO ops_quota_configs
	(id, plan_id, max_projects, max_deployments, max_concurrent_builds, max_concurrent_deploys,
	 max_containers, max_custom_domains, max_storage_gb, max_bandwidth_gb_month,
	 max_build_minutes_month, max_container_hours_month, max_api_requests_day)
VALUES
	(gen_random_uuid(), 'free',       3,    10,   1,   1,   3,   1,    1.0,    100,    500,   720,   100000),
	(gen_random_uuid(), 'pro',        20,   100,  3,   3,   20,  10,   20.0,   1000,   2000,  7200,  1000000),
	(gen_random_uuid(), 'team',       100,  1000, 10,  10,  100, 50,   100.0,  10000,  10000, 72000, 10000000),
	(gen_random_uuid(), 'enterprise', -1,   -1,   -1,  -1,  -1,  -1,   -1,     -1,     -1,    -1,    -1)
ON CONFLICT (plan_id) DO NOTHING;

CREATE TABLE IF NOT EXISTS ops_usage_records (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL,
	project_id     UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	deployment_id  UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	dimension      TEXT NOT NULL,
	quantity       NUMERIC(20,6) NOT NULL DEFAULT 0,
	unit           TEXT NOT NULL DEFAULT '',
	period         TEXT NOT NULL,
	correlation_id TEXT NOT NULL DEFAULT '',
	recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_usage_records_org_period_idx ON ops_usage_records (org_id, period);
CREATE INDEX IF NOT EXISTS ops_usage_records_dim_idx ON ops_usage_records (dimension, period);

CREATE TABLE IF NOT EXISTS ops_usage_rollups (
	id         UUID PRIMARY KEY,
	org_id     UUID NOT NULL,
	project_id UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	dimension  TEXT NOT NULL,
	period     TEXT NOT NULL,
	total      NUMERIC(20,6) NOT NULL DEFAULT 0,
	unit       TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (org_id, project_id, dimension, period)
);

CREATE INDEX IF NOT EXISTS ops_usage_rollups_org_idx ON ops_usage_rollups (org_id, period);

CREATE TABLE IF NOT EXISTS ops_notifications (
	id              UUID PRIMARY KEY,
	org_id          UUID NOT NULL,
	user_id         UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	project_id      UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	channel         TEXT NOT NULL,
	event_type      TEXT NOT NULL,
	subject         TEXT NOT NULL DEFAULT '',
	body            TEXT NOT NULL DEFAULT '',
	recipient       TEXT NOT NULL DEFAULT '',
	status          TEXT NOT NULL DEFAULT 'pending',
	attempts        INT NOT NULL DEFAULT 0,
	last_attempt_at TIMESTAMPTZ,
	delivered_at    TIMESTAMPTZ,
	failure_reason  TEXT NOT NULL DEFAULT '',
	correlation_id  TEXT NOT NULL DEFAULT '',
	metadata        JSONB NOT NULL DEFAULT '{}'::jsonb,
	created_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_notifications_status_idx ON ops_notifications (status);
CREATE INDEX IF NOT EXISTS ops_notifications_org_idx ON ops_notifications (org_id, created_at DESC);

CREATE TABLE IF NOT EXISTS ops_notification_prefs (
	id         UUID PRIMARY KEY,
	org_id     UUID NOT NULL,
	user_id    UUID NOT NULL,
	channel    TEXT NOT NULL,
	event_type TEXT NOT NULL,
	enabled    BOOLEAN NOT NULL DEFAULT true,
	endpoint   TEXT NOT NULL DEFAULT '',
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (org_id, user_id, channel, event_type)
);

CREATE TABLE IF NOT EXISTS ops_backups (
	id               UUID PRIMARY KEY,
	kind             TEXT NOT NULL DEFAULT 'database',
	status           TEXT NOT NULL DEFAULT 'pending',
	size_bytes       BIGINT NOT NULL DEFAULT 0,
	storage_path     TEXT NOT NULL DEFAULT '',
	checksum         TEXT NOT NULL DEFAULT '',
	duration_seconds BIGINT NOT NULL DEFAULT 0,
	retention_days   INT NOT NULL DEFAULT 30,
	expires_at       TIMESTAMPTZ,
	verified_at      TIMESTAMPTZ,
	failure_reason   TEXT NOT NULL DEFAULT '',
	correlation_id   TEXT NOT NULL DEFAULT '',
	metadata         JSONB NOT NULL DEFAULT '{}'::jsonb,
	started_at       TIMESTAMPTZ,
	completed_at     TIMESTAMPTZ,
	created_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_backups_status_idx ON ops_backups (status, created_at DESC);
CREATE INDEX IF NOT EXISTS ops_backups_expires_idx ON ops_backups (expires_at) WHERE status='completed';

CREATE TABLE IF NOT EXISTS ops_analytics_daily (
	id                    UUID PRIMARY KEY,
	period                TEXT NOT NULL UNIQUE,
	active_users          BIGINT NOT NULL DEFAULT 0,
	new_orgs              BIGINT NOT NULL DEFAULT 0,
	new_projects          BIGINT NOT NULL DEFAULT 0,
	total_deployments     BIGINT NOT NULL DEFAULT 0,
	success_deployments   BIGINT NOT NULL DEFAULT 0,
	failed_deployments    BIGINT NOT NULL DEFAULT 0,
	total_builds          BIGINT NOT NULL DEFAULT 0,
	success_builds        BIGINT NOT NULL DEFAULT 0,
	failed_builds         BIGINT NOT NULL DEFAULT 0,
	avg_build_duration_secs BIGINT NOT NULL DEFAULT 0,
	active_containers     BIGINT NOT NULL DEFAULT 0,
	total_bandwidth_gb    NUMERIC(14,3) NOT NULL DEFAULT 0,
	total_storage_gb      NUMERIC(14,3) NOT NULL DEFAULT 0,
	total_api_requests    BIGINT NOT NULL DEFAULT 0,
	new_subscriptions     BIGINT NOT NULL DEFAULT 0,
	mrr_cents             BIGINT NOT NULL DEFAULT 0,
	created_at            TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at            TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS ops_audit_events (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	project_id     UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	actor_id       UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	actor_type     TEXT NOT NULL DEFAULT 'user',
	action         TEXT NOT NULL,
	resource_type  TEXT NOT NULL DEFAULT '',
	resource_id    TEXT NOT NULL DEFAULT '',
	ip_address     TEXT NOT NULL DEFAULT '',
	user_agent     TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	changes        JSONB NOT NULL DEFAULT '{}'::jsonb,
	metadata       JSONB NOT NULL DEFAULT '{}'::jsonb,
	occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_audit_events_org_idx ON ops_audit_events (org_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS ops_audit_events_actor_idx ON ops_audit_events (actor_id, occurred_at DESC);
CREATE INDEX IF NOT EXISTS ops_audit_events_action_idx ON ops_audit_events (action, occurred_at DESC);

CREATE TABLE IF NOT EXISTS ops_sleep_events (
	id             UUID PRIMARY KEY,
	org_id         UUID NOT NULL,
	project_id     UUID NOT NULL,
	deployment_id  UUID NOT NULL DEFAULT '00000000-0000-0000-0000-000000000000',
	status         TEXT NOT NULL DEFAULT 'sleeping',
	reason         TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_sleep_events_project_idx ON ops_sleep_events (project_id, occurred_at DESC);

CREATE TABLE IF NOT EXISTS ops_cron_jobs (
	id             UUID PRIMARY KEY,
	name           TEXT NOT NULL UNIQUE,
	schedule       TEXT NOT NULL,
	timezone       TEXT NOT NULL DEFAULT 'UTC',
	job_queue      TEXT NOT NULL DEFAULT 'ops',
	job_type       TEXT NOT NULL,
	payload        JSONB NOT NULL DEFAULT '{}'::jsonb,
	status         TEXT NOT NULL DEFAULT 'active',
	last_run_at    TIMESTAMPTZ,
	next_run_at    TIMESTAMPTZ,
	last_error     TEXT NOT NULL DEFAULT '',
	correlation_id TEXT NOT NULL DEFAULT '',
	created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX IF NOT EXISTS ops_cron_jobs_next_run_idx ON ops_cron_jobs (next_run_at) WHERE status='active';
`

// Migrations returns ops migrations chained after proxy (0008).
func Migrations() []postgres.Migration {
	m := proxy.Migrations()
	m = append(m, postgres.Migration{Version: "0009_ops", SQL: SchemaDDL})
	return m
}
