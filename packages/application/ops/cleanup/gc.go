// Package cleanup implements garbage collection of orphaned and expired platform resources.
// Each cleaner is independently configurable and skippable via feature flags.
package cleanup

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"go.uber.org/zap"
)

// Target labels each GC job.
type Target string

const (
	TargetExpiredSessions  Target = "sessions"
	TargetExpiredTokens    Target = "tokens"
	TargetOldLogs          Target = "logs"
	TargetExpiredJobs      Target = "dead_letter_jobs"
	TargetOldNotifications Target = "notifications"
	TargetExpiredBackups   Target = "expired_backups"
	TargetStaleUsage       Target = "stale_usage_records"
	TargetOldAuditEvents   Target = "old_audit_events"
)

// Config controls retention windows for each target.
type Config struct {
	SessionMaxAge      time.Duration
	TokenMaxAge        time.Duration
	LogMaxAge          time.Duration
	JobDeadLetterAge   time.Duration
	NotificationMaxAge time.Duration
	AuditRetention     time.Duration
	UsageRawRetention  time.Duration
}

// DefaultConfig returns conservative retention defaults.
func DefaultConfig() Config {
	return Config{
		SessionMaxAge:      7 * 24 * time.Hour,
		TokenMaxAge:        24 * time.Hour,
		LogMaxAge:          30 * 24 * time.Hour,
		JobDeadLetterAge:   7 * 24 * time.Hour,
		NotificationMaxAge: 90 * 24 * time.Hour,
		AuditRetention:     365 * 24 * time.Hour,
		UsageRawRetention:  90 * 24 * time.Hour,
	}
}

// GC executes all cleanup tasks and returns a summary of rows purged per target.
type GC struct {
	db  *postgres.DB
	cfg Config
	log *zap.Logger
}

// NewGC constructs a garbage collector.
func NewGC(db *postgres.DB, cfg Config, log *zap.Logger) *GC {
	return &GC{db: db, cfg: cfg, log: log}
}

// RunAll executes every cleanup task and returns total rows purged.
func (g *GC) RunAll(ctx context.Context) map[Target]int64 {
	results := make(map[Target]int64)
	tasks := []struct {
		target Target
		fn     func(context.Context) (int64, error)
	}{
		{TargetExpiredSessions, g.cleanSessions},
		{TargetExpiredTokens, g.cleanTokens},
		{TargetExpiredJobs, g.cleanDeadLetterJobs},
		{TargetOldNotifications, g.cleanNotifications},
		{TargetOldAuditEvents, g.cleanAuditEvents},
		{TargetStaleUsage, g.cleanRawUsage},
	}
	for _, t := range tasks {
		n, err := t.fn(ctx)
		if err != nil {
			g.log.Warn("gc: task failed",
				zap.String("target", string(t.target)),
				zap.Error(err))
		} else {
			results[t.target] = n
			if n > 0 {
				g.log.Info("gc: purged",
					zap.String("target", string(t.target)),
					zap.Int64("rows", n))
			}
		}
	}
	return results
}

func (g *GC) cleanSessions(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.SessionMaxAge)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM identity_sessions WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: sessions")
	}
	return tag.RowsAffected(), nil
}

func (g *GC) cleanTokens(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.TokenMaxAge)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM identity_refresh_tokens WHERE expires_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: tokens")
	}
	return tag.RowsAffected(), nil
}

func (g *GC) cleanDeadLetterJobs(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.JobDeadLetterAge)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM jobs WHERE status='dead' AND updated_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: dead jobs")
	}
	return tag.RowsAffected(), nil
}

func (g *GC) cleanNotifications(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.NotificationMaxAge)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM ops_notifications WHERE status IN ('delivered','skipped') AND updated_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: notifications")
	}
	return tag.RowsAffected(), nil
}

func (g *GC) cleanAuditEvents(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.AuditRetention)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM ops_audit_events WHERE occurred_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: audit events")
	}
	return tag.RowsAffected(), nil
}

func (g *GC) cleanRawUsage(ctx context.Context) (int64, error) {
	cutoff := time.Now().UTC().Add(-g.cfg.UsageRawRetention)
	tag, err := g.db.Conn(ctx).Exec(ctx,
		`DELETE FROM ops_usage_records WHERE recorded_at < $1`, cutoff)
	if err != nil {
		return 0, postgres.Translate(err, "gc: raw usage")
	}
	return tag.RowsAffected(), nil
}

// RunTarget executes a single named cleanup task.
func (g *GC) RunTarget(ctx context.Context, target Target) (int64, error) {
	switch target {
	case TargetExpiredSessions:
		return g.cleanSessions(ctx)
	case TargetExpiredTokens:
		return g.cleanTokens(ctx)
	case TargetExpiredJobs:
		return g.cleanDeadLetterJobs(ctx)
	case TargetOldNotifications:
		return g.cleanNotifications(ctx)
	case TargetOldAuditEvents:
		return g.cleanAuditEvents(ctx)
	case TargetStaleUsage:
		return g.cleanRawUsage(ctx)
	default:
		return 0, nil
	}
}
