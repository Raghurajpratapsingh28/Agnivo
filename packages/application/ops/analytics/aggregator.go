// Package analytics aggregates platform-wide metrics into daily snapshots
// suitable for dashboards, growth tracking, and revenue reporting.
package analytics

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/model"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"go.uber.org/zap"
)

// Aggregator computes and persists daily analytics snapshots.
type Aggregator struct {
	repo *store.Repository
	db   *postgres.DB
	log  *zap.Logger
}

// NewAggregator constructs an Aggregator.
func NewAggregator(repo *store.Repository, db *postgres.DB, log *zap.Logger) *Aggregator {
	return &Aggregator{repo: repo, db: db, log: log}
}

// AggregateDaily computes the daily analytics snapshot for the given period (YYYY-MM-DD).
func (a *Aggregator) AggregateDaily(ctx context.Context, period string) error {
	if period == "" {
		period = time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	}

	snap := model.AnalyticsDaily{Period: period}

	// Active users: identity sessions touched during the period.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COUNT(DISTINCT user_id) FROM identity_sessions
WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(&snap.ActiveUsers)

	// New orgs.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COUNT(*) FROM identity_organizations
WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(&snap.NewOrgs)

	// New projects.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COUNT(*) FROM controlplane_projects
WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(&snap.NewProjects)

	// Deployments.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT
	COUNT(*),
	COUNT(*) FILTER (WHERE status='success'),
	COUNT(*) FILTER (WHERE status='failed')
FROM controlplane_deployments
WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(
		&snap.TotalDeployments, &snap.SuccessDeployments, &snap.FailedDeployments)

	// Builds.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT
	COUNT(*),
	COUNT(*) FILTER (WHERE status='success'),
	COUNT(*) FILTER (WHERE status='failed'),
	COALESCE(AVG(EXTRACT(EPOCH FROM (completed_at - started_at))), 0)
FROM builder_builds
WHERE DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(
		&snap.TotalBuilds, &snap.SuccessBuilds, &snap.FailedBuilds, &snap.AvgBuildDurationSecs)

	// Active containers (count at end of period).
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COUNT(*) FROM runtime_containers WHERE status='running'`).Scan(&snap.ActiveContainers)

	// Bandwidth & storage from usage rollups.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(total), 0) FROM ops_usage_rollups WHERE dimension='bandwidth_gb' AND period=$1`, period).Scan(&snap.TotalBandwidthGB)
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(total), 0) FROM ops_usage_rollups WHERE dimension='storage_gb' AND period=$1`, period).Scan(&snap.TotalStorageGB)

	// API requests.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(total), 0) FROM ops_usage_rollups WHERE dimension='api_requests' AND period=$1`, period).Scan(&snap.TotalAPIRequests)

	// New subscriptions (plan != free).
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COUNT(*) FROM ops_subscriptions WHERE plan_id != 'free' AND DATE(created_at AT TIME ZONE 'UTC') = $1::DATE`, period).Scan(&snap.NewSubscriptions)

	// MRR: sum of monthly price for all active non-free subscriptions.
	_ = a.db.Conn(ctx).QueryRow(ctx, `
SELECT COALESCE(SUM(p.price_cents_month), 0)
FROM ops_subscriptions s
JOIN ops_plans p ON p.id = s.plan_id
WHERE s.status IN ('active','trialing') AND s.plan_id != 'free'`).Scan(&snap.MRR)

	if err := a.repo.UpsertAnalyticsDaily(ctx, snap); err != nil {
		return err
	}
	a.log.Info("analytics: daily aggregation complete",
		zap.String("period", period),
		zap.Int64("active_users", snap.ActiveUsers),
		zap.Int64("deployments", snap.TotalDeployments))
	return nil
}

// AggregateYesterday is a convenience wrapper for nightly cron.
func (a *Aggregator) AggregateYesterday(ctx context.Context) error {
	yesterday := time.Now().UTC().AddDate(0, 0, -1).Format("2006-01-02")
	return a.AggregateDaily(ctx, yesterday)
}
