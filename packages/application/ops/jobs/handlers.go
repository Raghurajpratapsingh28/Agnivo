// Package jobs registers all background job handlers for the Platform Operations worker.
package jobs

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/analytics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/autosleep"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/backup"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/billing"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/cleanup"
	opsevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/metering"
	opsmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/notification"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"go.uber.org/zap"
)

// Queue names for the ops worker.
const (
	QueueOps           = "ops"
	QueueNotifications = "notifications"
	QueueBackup        = "backup"
	QueueCleanup       = "cleanup"
	QueueAnalytics     = "analytics"
	QueueBilling       = "billing"
)

// Ops job type names.
const (
	TypeUsageRollup  = "ops.usage.rollup"
	TypeNotify       = "ops.notify"
	TypeBackupDB     = "ops.backup.database"
	TypeCleanup      = "ops.cleanup"
	TypeAnalytics    = "ops.analytics.daily"
	TypeBillingCycle = "ops.billing.cycle"
	TypeAutoSleep    = autosleep.TypeSleep
)

// Handlers holds all job handler dependencies.
type Handlers struct {
	billing   *billing.Engine
	metering  *metering.Collector
	notifier  *notification.Dispatcher
	backupMgr *backup.Manager
	gc        *cleanup.GC
	analytics *analytics.Aggregator
	sleepMgr  *autosleep.Manager
	pub       *opsevents.Publisher
	metrics   *opsmetrics.Metrics
	log       *zap.Logger
}

// NewHandlers constructs job Handlers.
func NewHandlers(
	billing *billing.Engine,
	metering *metering.Collector,
	notifier *notification.Dispatcher,
	backupMgr *backup.Manager,
	gc *cleanup.GC,
	analytics *analytics.Aggregator,
	sleepMgr *autosleep.Manager,
	pub *opsevents.Publisher,
	metrics *opsmetrics.Metrics,
	log *zap.Logger,
) *Handlers {
	return &Handlers{
		billing:   billing,
		metering:  metering,
		notifier:  notifier,
		backupMgr: backupMgr,
		gc:        gc,
		analytics: analytics,
		sleepMgr:  sleepMgr,
		pub:       pub,
		metrics:   metrics,
		log:       log,
	}
}

// Register attaches all handlers to the provided Worker.
func (h *Handlers) Register(w *jobs.Worker) {
	w.Handle(TypeUsageRollup, h.handleUsageRollup)
	w.Handle(TypeNotify, h.handleNotify)
	w.Handle(TypeBackupDB, h.handleBackupDB)
	w.Handle(TypeCleanup, h.handleCleanup)
	w.Handle(TypeAnalytics, h.handleAnalytics)
	w.Handle(TypeBillingCycle, h.handleBillingCycle)
	w.Handle(TypeAutoSleep, h.handleAutoSleep)
}

// ─────────────────────────────── Handlers ────────────────────────────────────

func (h *Handlers) handleUsageRollup(ctx context.Context, j jobs.Job) error {
	var p model.UsageRollupPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	start := time.Now()
	err := h.metering.Rollup(ctx, p.Period)
	h.metrics.RollupDurationMs.Observe(float64(time.Since(start).Milliseconds()))
	if err == nil {
		_ = h.pub.PublishAsync(ctx, opsevents.UsageUpdated,
			opsevents.Meta{CorrelationID: p.CorrelationID},
			map[string]string{"period": p.Period})
	}
	return err
}

func (h *Handlers) handleNotify(ctx context.Context, j jobs.Job) error {
	var p model.NotifyPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	start := time.Now()
	err := h.notifier.Deliver(ctx, p.NotificationID)
	ms := float64(time.Since(start).Milliseconds())
	if err != nil {
		h.metrics.NotifFailed.WithLabelValues("unknown").Inc()
		_ = h.pub.PublishAsync(ctx, opsevents.NotificationFailed,
			opsevents.Meta{CorrelationID: p.CorrelationID},
			map[string]string{"notification_id": p.NotificationID, "error": err.Error()})
		return err
	}
	h.metrics.NotifDelivered.WithLabelValues("unknown").Inc()
	h.metrics.NotifLatencyMs.WithLabelValues("unknown").Observe(ms)
	_ = h.pub.PublishAsync(ctx, opsevents.NotificationDelivered,
		opsevents.Meta{CorrelationID: p.CorrelationID},
		map[string]string{"notification_id": p.NotificationID})
	return nil
}

func (h *Handlers) handleBackupDB(ctx context.Context, j jobs.Job) error {
	var p model.BackupPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	start := time.Now()
	bk, err := h.backupMgr.RunDatabaseBackup(ctx, p.CorrelationID)
	ms := float64(time.Since(start).Milliseconds())
	h.metrics.BackupDurationMs.Observe(ms)
	if err != nil {
		h.metrics.BackupRuns.WithLabelValues(string(p.Kind), "failed").Inc()
		_ = h.pub.PublishAsync(ctx, opsevents.BackupFailed,
			opsevents.Meta{CorrelationID: p.CorrelationID},
			map[string]string{"error": err.Error()})
		return err
	}
	h.metrics.BackupRuns.WithLabelValues(string(bk.Kind), "completed").Inc()
	h.metrics.BackupSizeBytes.Observe(float64(bk.SizeBytes))
	_ = h.pub.PublishAsync(ctx, opsevents.BackupCompleted,
		opsevents.Meta{CorrelationID: p.CorrelationID},
		map[string]any{"backup_id": bk.ID, "size_bytes": bk.SizeBytes})
	return nil
}

func (h *Handlers) handleCleanup(ctx context.Context, j jobs.Job) error {
	var p model.CleanupPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	start := time.Now()
	var results map[cleanup.Target]int64
	if p.Target != "" {
		n, err := h.gc.RunTarget(ctx, cleanup.Target(p.Target))
		if err != nil {
			return err
		}
		results = map[cleanup.Target]int64{cleanup.Target(p.Target): n}
	} else {
		results = h.gc.RunAll(ctx)
	}
	h.metrics.CleanupRuns.Inc()
	for target, n := range results {
		h.metrics.RowsPurged.WithLabelValues(string(target)).Add(float64(n))
	}
	h.log.Info("gc: cleanup complete",
		zap.Duration("duration", time.Since(start)))
	_ = h.pub.PublishAsync(ctx, opsevents.GarbageCollected,
		opsevents.Meta{CorrelationID: p.CorrelationID}, results)
	return nil
}

func (h *Handlers) handleAnalytics(ctx context.Context, j jobs.Job) error {
	var p model.AnalyticsPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	start := time.Now()
	err := h.analytics.AggregateDaily(ctx, p.Period)
	h.metrics.AnalyticsDurationMs.Observe(float64(time.Since(start).Milliseconds()))
	h.metrics.AnalyticsRuns.Inc()
	if err == nil {
		_ = h.pub.PublishAsync(ctx, opsevents.AnalyticsUpdated,
			opsevents.Meta{CorrelationID: p.CorrelationID},
			map[string]string{"period": p.Period})
	}
	return err
}

func (h *Handlers) handleBillingCycle(ctx context.Context, j jobs.Job) error {
	var p model.BillingPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	sub, err := h.billing.GenerateInvoice(ctx, p.OrgID, 0, p.CorrelationID)
	if err != nil {
		h.metrics.BillingErrors.Inc()
		return err
	}
	h.metrics.InvoicesGenerated.Inc()
	_ = h.pub.PublishAsync(ctx, opsevents.InvoiceGenerated,
		opsevents.Meta{OrgID: p.OrgID, CorrelationID: p.CorrelationID},
		map[string]any{"invoice_id": sub.ID})
	return nil
}

func (h *Handlers) handleAutoSleep(ctx context.Context, j jobs.Job) error {
	var p model.SleepPayload
	if err := j.Decode(&p); err != nil {
		return err
	}
	if err := h.sleepMgr.ExecuteSleep(ctx, p); err != nil {
		return err
	}
	h.metrics.SleepEvents.Inc()
	_ = h.pub.PublishAsync(ctx, opsevents.ProjectSleeping,
		opsevents.Meta{OrgID: p.OrgID, ProjectID: p.ProjectID, DeploymentID: p.DeploymentID, CorrelationID: p.CorrelationID},
		nil)
	return nil
}
