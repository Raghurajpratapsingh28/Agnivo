// Package store provides all database persistence for the Platform Operations Layer.
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/jackc/pgx/v5"
)

// Repository provides CRUD for every ops subsystem.
type Repository struct{ db *postgres.DB }

// NewRepository constructs the ops Repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// ──────────────────────────────── Subscriptions ──────────────────────────────

// GetSubscription returns the subscription for an org (creates a free one if absent).
func (r *Repository) GetSubscription(ctx context.Context, orgID string) (model.Subscription, error) {
	const q = `SELECT id, org_id, plan_id, status, interval, stripe_sub_id, stripe_customer_id,
		current_period_start, current_period_end, trial_ends_at, canceled_at,
		grace_period_ends_at, correlation_id, metadata, created_at, updated_at
		FROM ops_subscriptions WHERE org_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, orgID)
	return scanSubscription(row)
}

// UpsertSubscription creates or updates a subscription.
func (r *Repository) UpsertSubscription(ctx context.Context, s model.Subscription) (model.Subscription, error) {
	if s.ID == "" {
		s.ID = idx.NewUUID()
	}
	if s.Metadata == nil {
		s.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO ops_subscriptions
	(id, org_id, plan_id, status, interval, stripe_sub_id, stripe_customer_id,
	 current_period_start, current_period_end, trial_ends_at, canceled_at,
	 grace_period_ends_at, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,now(),now())
ON CONFLICT (org_id) DO UPDATE SET
	plan_id=EXCLUDED.plan_id, status=EXCLUDED.status, interval=EXCLUDED.interval,
	stripe_sub_id=EXCLUDED.stripe_sub_id, stripe_customer_id=EXCLUDED.stripe_customer_id,
	current_period_start=EXCLUDED.current_period_start,
	current_period_end=EXCLUDED.current_period_end,
	trial_ends_at=EXCLUDED.trial_ends_at, canceled_at=EXCLUDED.canceled_at,
	grace_period_ends_at=EXCLUDED.grace_period_ends_at,
	correlation_id=EXCLUDED.correlation_id, metadata=EXCLUDED.metadata, updated_at=now()
RETURNING id, org_id, plan_id, status, interval, stripe_sub_id, stripe_customer_id,
	current_period_start, current_period_end, trial_ends_at, canceled_at,
	grace_period_ends_at, correlation_id, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		s.ID, s.OrgID, s.PlanID, s.Status, s.Interval, s.StripeSubID, s.StripeCustomerID,
		s.CurrentPeriodStart, s.CurrentPeriodEnd, s.TrialEndsAt, s.CanceledAt,
		s.GracePeriodEndsAt, s.CorrelationID, s.Metadata)
	return scanSubscription(row)
}

// ListSubscriptionsDueForRenewal returns active subscriptions whose period ends within the next 24h.
func (r *Repository) ListSubscriptionsDueForRenewal(ctx context.Context) ([]model.Subscription, error) {
	const q = `SELECT id, org_id, plan_id, status, interval, stripe_sub_id, stripe_customer_id,
		current_period_start, current_period_end, trial_ends_at, canceled_at,
		grace_period_ends_at, correlation_id, metadata, created_at, updated_at
		FROM ops_subscriptions
		WHERE status='active' AND current_period_end <= now() + INTERVAL '24 hours'
		ORDER BY current_period_end`
	rows, err := r.db.Conn(ctx).Query(ctx, q)
	if err != nil {
		return nil, postgres.Translate(err, "ops: list renewals")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Subscription])
}

// ──────────────────────────────── Invoices ───────────────────────────────────

// InsertInvoice creates a new invoice.
func (r *Repository) InsertInvoice(ctx context.Context, inv model.Invoice) (model.Invoice, error) {
	if inv.ID == "" {
		inv.ID = idx.NewUUID()
	}
	if inv.Metadata == nil {
		inv.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO ops_invoices
	(id, org_id, subscription_id, status, period_start, period_end,
	 amount_cents, credits_cents, tax_cents, total_cents, currency,
	 stripe_inv_id, paid_at, due_at, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,now(),now())
RETURNING id, org_id, subscription_id, status, period_start, period_end,
	amount_cents, credits_cents, tax_cents, total_cents, currency,
	stripe_inv_id, paid_at, due_at, correlation_id, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		inv.ID, inv.OrgID, inv.SubscriptionID, inv.Status, inv.PeriodStart, inv.PeriodEnd,
		inv.AmountCents, inv.CreditsCents, inv.TaxCents, inv.TotalCents, inv.Currency,
		inv.StripeInvID, inv.PaidAt, inv.DueAt, inv.CorrelationID, inv.Metadata)
	return scanInvoice(row)
}

// SetInvoiceStatus updates an invoice status.
func (r *Repository) SetInvoiceStatus(ctx context.Context, id string, status model.InvoiceStatus, paidAt *time.Time) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE ops_invoices SET status=$2, paid_at=$3, updated_at=now() WHERE id=$1`,
		id, status, paidAt)
	return postgres.Translate(err, "ops: set invoice status")
}

// ListInvoicesByOrg returns recent invoices for an org.
func (r *Repository) ListInvoicesByOrg(ctx context.Context, orgID string, limit int) ([]model.Invoice, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, org_id, subscription_id, status, period_start, period_end,
	amount_cents, credits_cents, tax_cents, total_cents, currency,
	stripe_inv_id, paid_at, due_at, correlation_id, metadata, created_at, updated_at
FROM ops_invoices WHERE org_id=$1 ORDER BY created_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "ops: list invoices")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Invoice])
}

// ──────────────────────────────── Credits ────────────────────────────────────

// AddCredit inserts a new credit grant.
func (r *Repository) AddCredit(ctx context.Context, c model.Credit) (model.Credit, error) {
	if c.ID == "" {
		c.ID = idx.NewUUID()
	}
	const q = `
INSERT INTO ops_credits (id, org_id, amount_cents, used_cents, reason, coupon_code, expires_at, correlation_id, created_at)
VALUES ($1,$2,$3,0,$4,$5,$6,$7,now())
RETURNING id, org_id, amount_cents, used_cents, reason, coupon_code, expires_at, correlation_id, created_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		c.ID, c.OrgID, c.AmountCents, c.Reason, c.CouponCode, c.ExpiresAt, c.CorrelationID)
	return scanCredit(row)
}

// AvailableCredits returns the total usable credits for an org.
func (r *Repository) AvailableCredits(ctx context.Context, orgID string) (int64, error) {
	var total int64
	err := r.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(amount_cents - used_cents), 0)
FROM ops_credits
WHERE org_id=$1 AND (expires_at IS NULL OR expires_at > now())`, orgID).Scan(&total)
	return total, postgres.Translate(err, "ops: available credits")
}

// ──────────────────────────────── Quota configs ──────────────────────────────

// GetQuotaConfig returns the quota limits for a plan.
func (r *Repository) GetQuotaConfig(ctx context.Context, planID model.PlanID) (model.QuotaConfig, error) {
	const q = `SELECT id, plan_id, max_projects, max_deployments, max_concurrent_builds,
		max_concurrent_deploys, max_containers, max_custom_domains,
		max_storage_gb, max_bandwidth_gb_month, max_build_minutes_month,
		max_container_hours_month, max_api_requests_day, warn_threshold_pct,
		created_at, updated_at
		FROM ops_quota_configs WHERE plan_id=$1`
	row := r.db.Conn(ctx).QueryRow(ctx, q, planID)
	var c model.QuotaConfig
	err := row.Scan(&c.ID, &c.PlanID, &c.MaxProjects, &c.MaxDeployments,
		&c.MaxConcurrentBuilds, &c.MaxConcurrentDeploys, &c.MaxContainers,
		&c.MaxCustomDomains, &c.MaxStorageGB, &c.MaxBandwidthGBMonth,
		&c.MaxBuildMinutesMonth, &c.MaxContainerHoursMonth, &c.MaxAPIRequestsDay,
		&c.WarnThresholdPct, &c.CreatedAt, &c.UpdatedAt)
	if err != nil {
		return model.QuotaConfig{}, postgres.Translate(err, "ops: quota config")
	}
	return c, nil
}

// ──────────────────────────────── Usage records ──────────────────────────────

// RecordUsage inserts a usage measurement.
func (r *Repository) RecordUsage(ctx context.Context, u model.UsageRecord) error {
	if u.ID == "" {
		u.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO ops_usage_records
	(id, org_id, project_id, deployment_id, dimension, quantity, unit, period, correlation_id, recorded_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now())`,
		u.ID, u.OrgID, u.ProjectID, u.DeploymentID, u.Dimension, u.Quantity,
		u.Unit, u.Period, u.CorrelationID)
	return postgres.Translate(err, "ops: record usage")
}

// RollupUsage aggregates raw records into rollups for a given period.
func (r *Repository) RollupUsage(ctx context.Context, period string) error {
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO ops_usage_rollups (id, org_id, project_id, dimension, period, total, unit, created_at, updated_at)
SELECT gen_random_uuid(), org_id, project_id, dimension, period,
	SUM(quantity), MIN(unit), now(), now()
FROM ops_usage_records
WHERE period=$1
GROUP BY org_id, project_id, dimension, period
ON CONFLICT (org_id, project_id, dimension, period) DO UPDATE SET
	total=EXCLUDED.total, updated_at=now()`, period)
	return postgres.Translate(err, "ops: rollup usage")
}

// GetCurrentUsage returns the current period's rollup for an org/dimension.
func (r *Repository) GetCurrentUsage(ctx context.Context, orgID string, dimension model.UsageDimension, period string) (float64, error) {
	var total float64
	err := r.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(total), 0) FROM ops_usage_rollups
WHERE org_id=$1 AND dimension=$2 AND period=$3`, orgID, dimension, period).Scan(&total)
	return total, postgres.Translate(err, "ops: get current usage")
}

// ──────────────────────────────── Notifications ──────────────────────────────

// InsertNotification creates a notification for delivery.
func (r *Repository) InsertNotification(ctx context.Context, n model.Notification) (model.Notification, error) {
	if n.ID == "" {
		n.ID = idx.NewUUID()
	}
	if n.Metadata == nil {
		n.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO ops_notifications
	(id, org_id, user_id, project_id, channel, event_type, subject, body,
	 recipient, status, attempts, correlation_id, metadata, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,'pending',0,$10,$11,now(),now())
RETURNING id, org_id, user_id, project_id, channel, event_type, subject, body,
	recipient, status, attempts, last_attempt_at, delivered_at,
	failure_reason, correlation_id, metadata, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		n.ID, n.OrgID, n.UserID, n.ProjectID, n.Channel, n.EventType,
		n.Subject, n.Body, n.Recipient, n.CorrelationID, n.Metadata)
	return scanNotification(row)
}

// GetNotification returns a notification by ID.
func (r *Repository) GetNotification(ctx context.Context, id string) (model.Notification, error) {
	const q = `SELECT id, org_id, user_id, project_id, channel, event_type, subject, body,
		recipient, status, attempts, last_attempt_at, delivered_at,
		failure_reason, correlation_id, metadata, created_at, updated_at
		FROM ops_notifications WHERE id=$1`
	return scanNotification(r.db.Conn(ctx).QueryRow(ctx, q, id))
}

// MarkNotificationDelivered marks a notification as delivered.
func (r *Repository) MarkNotificationDelivered(ctx context.Context, id string) error {
	now := time.Now().UTC()
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE ops_notifications SET status='delivered', delivered_at=$2, updated_at=now() WHERE id=$1`, id, now)
	return postgres.Translate(err, "ops: mark delivered")
}

// MarkNotificationFailed records a delivery failure.
func (r *Repository) MarkNotificationFailed(ctx context.Context, id, reason string) error {
	now := time.Now().UTC()
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE ops_notifications SET status='failed', failure_reason=$2,
	last_attempt_at=$3, attempts=attempts+1, updated_at=now() WHERE id=$1`, id, reason, now)
	return postgres.Translate(err, "ops: mark failed")
}

// ListPendingNotifications returns notifications due for delivery.
func (r *Repository) ListPendingNotifications(ctx context.Context, limit int) ([]model.Notification, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, org_id, user_id, project_id, channel, event_type, subject, body,
	recipient, status, attempts, last_attempt_at, delivered_at,
	failure_reason, correlation_id, metadata, created_at, updated_at
FROM ops_notifications
WHERE status='pending' AND attempts < 5
ORDER BY created_at ASC LIMIT $1`, limit)
	if err != nil {
		return nil, postgres.Translate(err, "ops: list pending notifications")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Notification])
}

// ──────────────────────────────── Backups ────────────────────────────────────

// InsertBackup creates a backup record.
func (r *Repository) InsertBackup(ctx context.Context, b model.Backup) (model.Backup, error) {
	if b.ID == "" {
		b.ID = idx.NewUUID()
	}
	if b.Metadata == nil {
		b.Metadata, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO ops_backups
	(id, kind, status, size_bytes, storage_path, checksum, duration_seconds,
	 retention_days, expires_at, failure_reason, correlation_id, metadata, created_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,now())
RETURNING id, kind, status, size_bytes, storage_path, checksum, duration_seconds,
	retention_days, expires_at, verified_at, failure_reason, correlation_id, metadata,
	started_at, completed_at, created_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		b.ID, b.Kind, b.Status, b.SizeBytes, b.StoragePath, b.Checksum,
		b.DurationSeconds, b.RetentionDays, b.ExpiresAt, b.FailureReason,
		b.CorrelationID, b.Metadata)
	return scanBackup(row)
}

// SetBackupStatus updates backup status.
func (r *Repository) SetBackupStatus(ctx context.Context, id string, status model.BackupStatus, reason string) error {
	_, err := r.db.Conn(ctx).Exec(ctx,
		`UPDATE ops_backups SET status=$2, failure_reason=$3 WHERE id=$1`, id, status, reason)
	return postgres.Translate(err, "ops: set backup status")
}

// CompleteBackup marks a backup finished with its metadata.
func (r *Repository) CompleteBackup(ctx context.Context, id, storagePath, checksum string, sizeBytes, durationSecs int64) error {
	now := time.Now().UTC()
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE ops_backups SET
	status='completed', storage_path=$2, checksum=$3, size_bytes=$4,
	duration_seconds=$5, completed_at=$6
WHERE id=$1`, id, storagePath, checksum, sizeBytes, durationSecs, now)
	return postgres.Translate(err, "ops: complete backup")
}

// ListExpiredBackups returns backups past retention.
func (r *Repository) ListExpiredBackups(ctx context.Context) ([]model.Backup, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, kind, status, size_bytes, storage_path, checksum, duration_seconds,
	retention_days, expires_at, verified_at, failure_reason, correlation_id, metadata,
	started_at, completed_at, created_at
FROM ops_backups WHERE status='completed' AND expires_at <= now()`)
	if err != nil {
		return nil, postgres.Translate(err, "ops: list expired backups")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.Backup])
}

// ──────────────────────────────── Analytics ──────────────────────────────────

// UpsertAnalyticsDaily inserts or updates a daily analytics snapshot.
func (r *Repository) UpsertAnalyticsDaily(ctx context.Context, a model.AnalyticsDaily) error {
	if a.ID == "" {
		a.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO ops_analytics_daily
	(id, period, active_users, new_orgs, new_projects, total_deployments, success_deployments,
	 failed_deployments, total_builds, success_builds, failed_builds, avg_build_duration_secs,
	 active_containers, total_bandwidth_gb, total_storage_gb, total_api_requests,
	 new_subscriptions, mrr_cents, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15,$16,$17,$18,now(),now())
ON CONFLICT (period) DO UPDATE SET
	active_users=EXCLUDED.active_users, new_orgs=EXCLUDED.new_orgs,
	new_projects=EXCLUDED.new_projects, total_deployments=EXCLUDED.total_deployments,
	success_deployments=EXCLUDED.success_deployments, failed_deployments=EXCLUDED.failed_deployments,
	total_builds=EXCLUDED.total_builds, success_builds=EXCLUDED.success_builds,
	failed_builds=EXCLUDED.failed_builds, avg_build_duration_secs=EXCLUDED.avg_build_duration_secs,
	active_containers=EXCLUDED.active_containers, total_bandwidth_gb=EXCLUDED.total_bandwidth_gb,
	total_storage_gb=EXCLUDED.total_storage_gb, total_api_requests=EXCLUDED.total_api_requests,
	new_subscriptions=EXCLUDED.new_subscriptions, mrr_cents=EXCLUDED.mrr_cents, updated_at=now()`,
		a.ID, a.Period, a.ActiveUsers, a.NewOrgs, a.NewProjects, a.TotalDeployments,
		a.SuccessDeployments, a.FailedDeployments, a.TotalBuilds, a.SuccessBuilds,
		a.FailedBuilds, a.AvgBuildDurationSecs, a.ActiveContainers, a.TotalBandwidthGB,
		a.TotalStorageGB, a.TotalAPIRequests, a.NewSubscriptions, a.MRR)
	return postgres.Translate(err, "ops: upsert analytics")
}

// GetAnalyticsDaily returns the snapshot for a period.
func (r *Repository) GetAnalyticsDaily(ctx context.Context, period string) (model.AnalyticsDaily, error) {
	var a model.AnalyticsDaily
	err := r.db.Conn(ctx).QueryRow(ctx, `
SELECT id, period, active_users, new_orgs, new_projects, total_deployments, success_deployments,
	failed_deployments, total_builds, success_builds, failed_builds, avg_build_duration_secs,
	active_containers, total_bandwidth_gb, total_storage_gb, total_api_requests,
	new_subscriptions, mrr_cents, created_at, updated_at
FROM ops_analytics_daily WHERE period=$1`, period).Scan(
		&a.ID, &a.Period, &a.ActiveUsers, &a.NewOrgs, &a.NewProjects,
		&a.TotalDeployments, &a.SuccessDeployments, &a.FailedDeployments,
		&a.TotalBuilds, &a.SuccessBuilds, &a.FailedBuilds, &a.AvgBuildDurationSecs,
		&a.ActiveContainers, &a.TotalBandwidthGB, &a.TotalStorageGB, &a.TotalAPIRequests,
		&a.NewSubscriptions, &a.MRR, &a.CreatedAt, &a.UpdatedAt)
	if err != nil {
		return a, postgres.Translate(err, "ops: get analytics")
	}
	return a, nil
}

// ──────────────────────────────── Audit ──────────────────────────────────────

// RecordAuditEvent persists an immutable audit entry.
func (r *Repository) RecordAuditEvent(ctx context.Context, e model.AuditEvent) error {
	if e.ID == "" {
		e.ID = idx.NewUUID()
	}
	if e.Changes == nil {
		e.Changes, _ = json.Marshal(map[string]any{})
	}
	if e.Metadata == nil {
		e.Metadata, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO ops_audit_events
	(id, org_id, project_id, actor_id, actor_type, action, resource_type, resource_id,
	 ip_address, user_agent, correlation_id, changes, metadata, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,now())`,
		e.ID, e.OrgID, e.ProjectID, e.ActorID, e.ActorType, e.Action,
		e.ResourceType, e.ResourceID, e.IPAddress, e.UserAgent,
		e.CorrelationID, e.Changes, e.Metadata)
	return postgres.Translate(err, "ops: record audit event")
}

// ListAuditEvents queries audit events with filters.
func (r *Repository) ListAuditEvents(ctx context.Context, orgID string, limit int) ([]model.AuditEvent, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, org_id, project_id, actor_id, actor_type, action, resource_type, resource_id,
	ip_address, user_agent, correlation_id, changes, metadata, occurred_at
FROM ops_audit_events WHERE org_id=$1 ORDER BY occurred_at DESC LIMIT $2`, orgID, limit)
	if err != nil {
		return nil, postgres.Translate(err, "ops: list audit events")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.AuditEvent])
}

// ──────────────────────────────── Sleep events ───────────────────────────────

// RecordSleepEvent persists a sleep/wake event.
func (r *Repository) RecordSleepEvent(ctx context.Context, e model.SleepEvent) error {
	if e.ID == "" {
		e.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx, `
INSERT INTO ops_sleep_events (id, org_id, project_id, deployment_id, status, reason, correlation_id, occurred_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,now())`,
		e.ID, e.OrgID, e.ProjectID, e.DeploymentID, e.Status, e.Reason, e.CorrelationID)
	return postgres.Translate(err, "ops: record sleep event")
}

// ──────────────────────────────── Cron jobs ──────────────────────────────────

// GetCronJobsDue returns active cron jobs whose next_run_at is in the past.
func (r *Repository) GetCronJobsDue(ctx context.Context) ([]model.CronJob, error) {
	rows, err := r.db.Conn(ctx).Query(ctx, `
SELECT id, name, schedule, timezone, job_queue, job_type, payload, status,
	last_run_at, next_run_at, last_error, correlation_id, created_at, updated_at
FROM ops_cron_jobs
WHERE status='active' AND (next_run_at IS NULL OR next_run_at <= now())
ORDER BY next_run_at ASC NULLS FIRST`)
	if err != nil {
		return nil, postgres.Translate(err, "ops: get due cron jobs")
	}
	return pgx.CollectRows(rows, pgx.RowToStructByName[model.CronJob])
}

// UpdateCronJobAfterRun updates the last run and next run timestamps.
func (r *Repository) UpdateCronJobAfterRun(ctx context.Context, id string, nextRunAt time.Time, lastErr string) error {
	now := time.Now().UTC()
	_, err := r.db.Conn(ctx).Exec(ctx, `
UPDATE ops_cron_jobs SET last_run_at=$2, next_run_at=$3, last_error=$4, updated_at=now()
WHERE id=$1`, id, now, nextRunAt, lastErr)
	return postgres.Translate(err, "ops: update cron after run")
}

// UpsertCronJob registers or updates a cron job definition.
func (r *Repository) UpsertCronJob(ctx context.Context, j model.CronJob) (model.CronJob, error) {
	if j.ID == "" {
		j.ID = idx.NewUUID()
	}
	if j.Payload == nil {
		j.Payload, _ = json.Marshal(map[string]any{})
	}
	const q = `
INSERT INTO ops_cron_jobs
	(id, name, schedule, timezone, job_queue, job_type, payload, status,
	 correlation_id, created_at, updated_at)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,now(),now())
ON CONFLICT (name) DO UPDATE SET
	schedule=EXCLUDED.schedule, timezone=EXCLUDED.timezone,
	job_queue=EXCLUDED.job_queue, job_type=EXCLUDED.job_type,
	payload=EXCLUDED.payload, status=EXCLUDED.status, updated_at=now()
RETURNING id, name, schedule, timezone, job_queue, job_type, payload, status,
	last_run_at, next_run_at, last_error, correlation_id, created_at, updated_at`
	row := r.db.Conn(ctx).QueryRow(ctx, q,
		j.ID, j.Name, j.Schedule, j.Timezone, j.JobQueue, j.JobType,
		j.Payload, j.Status, j.CorrelationID)
	return scanCronJob(row)
}

// ─────────────────────────────── Scan helpers ────────────────────────────────

func scanSubscription(row pgx.Row) (model.Subscription, error) {
	var s model.Subscription
	err := row.Scan(
		&s.ID, &s.OrgID, &s.PlanID, &s.Status, &s.Interval, &s.StripeSubID, &s.StripeCustomerID,
		&s.CurrentPeriodStart, &s.CurrentPeriodEnd, &s.TrialEndsAt, &s.CanceledAt,
		&s.GracePeriodEndsAt, &s.CorrelationID, &s.Metadata, &s.CreatedAt, &s.UpdatedAt,
	)
	if err != nil {
		return model.Subscription{}, postgres.Translate(err, "ops: scan subscription")
	}
	return s, nil
}

func scanInvoice(row pgx.Row) (model.Invoice, error) {
	var inv model.Invoice
	err := row.Scan(
		&inv.ID, &inv.OrgID, &inv.SubscriptionID, &inv.Status,
		&inv.PeriodStart, &inv.PeriodEnd, &inv.AmountCents, &inv.CreditsCents,
		&inv.TaxCents, &inv.TotalCents, &inv.Currency, &inv.StripeInvID,
		&inv.PaidAt, &inv.DueAt, &inv.CorrelationID, &inv.Metadata, &inv.CreatedAt, &inv.UpdatedAt,
	)
	if err != nil {
		return model.Invoice{}, postgres.Translate(err, "ops: scan invoice")
	}
	return inv, nil
}

func scanCredit(row pgx.Row) (model.Credit, error) {
	var c model.Credit
	err := row.Scan(&c.ID, &c.OrgID, &c.AmountCents, &c.UsedCents, &c.Reason, &c.CouponCode, &c.ExpiresAt, &c.CorrelationID, &c.CreatedAt)
	if err != nil {
		return model.Credit{}, postgres.Translate(err, "ops: scan credit")
	}
	return c, nil
}

func scanNotification(row pgx.Row) (model.Notification, error) {
	var n model.Notification
	err := row.Scan(
		&n.ID, &n.OrgID, &n.UserID, &n.ProjectID, &n.Channel, &n.EventType,
		&n.Subject, &n.Body, &n.Recipient, &n.Status, &n.Attempts,
		&n.LastAttemptAt, &n.DeliveredAt, &n.FailureReason, &n.CorrelationID,
		&n.Metadata, &n.CreatedAt, &n.UpdatedAt,
	)
	if err != nil {
		return model.Notification{}, postgres.Translate(err, "ops: scan notification")
	}
	return n, nil
}

func scanBackup(row pgx.Row) (model.Backup, error) {
	var b model.Backup
	err := row.Scan(
		&b.ID, &b.Kind, &b.Status, &b.SizeBytes, &b.StoragePath, &b.Checksum,
		&b.DurationSeconds, &b.RetentionDays, &b.ExpiresAt, &b.VerifiedAt,
		&b.FailureReason, &b.CorrelationID, &b.Metadata, &b.StartedAt, &b.CompletedAt, &b.CreatedAt,
	)
	if err != nil {
		return model.Backup{}, postgres.Translate(err, "ops: scan backup")
	}
	return b, nil
}

func scanCronJob(row pgx.Row) (model.CronJob, error) {
	var j model.CronJob
	err := row.Scan(
		&j.ID, &j.Name, &j.Schedule, &j.Timezone, &j.JobQueue, &j.JobType,
		&j.Payload, &j.Status, &j.LastRunAt, &j.NextRunAt, &j.LastError,
		&j.CorrelationID, &j.CreatedAt, &j.UpdatedAt,
	)
	if err != nil {
		return model.CronJob{}, postgres.Translate(err, "ops: scan cron job")
	}
	return j, nil
}
