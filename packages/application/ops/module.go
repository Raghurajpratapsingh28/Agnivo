// Package ops is the Platform Operations composition root.
// It wires all subsystems — billing, metering, quota, notifications, backup,
// cleanup, analytics, audit, auto-sleep, cron, and events — into a single
// Module that the worker and cron executables register.
package ops

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/ops/analytics"
	"github.com/agnivo/agnivo/packages/application/ops/audit"
	"github.com/agnivo/agnivo/packages/application/ops/autosleep"
	"github.com/agnivo/agnivo/packages/application/ops/backup"
	"github.com/agnivo/agnivo/packages/application/ops/billing"
	"github.com/agnivo/agnivo/packages/application/ops/cleanup"
	"github.com/agnivo/agnivo/packages/application/ops/cron"
	opsevents "github.com/agnivo/agnivo/packages/application/ops/events"
	opsjobs "github.com/agnivo/agnivo/packages/application/ops/jobs"
	opsmetrics "github.com/agnivo/agnivo/packages/application/ops/metrics"
	"github.com/agnivo/agnivo/packages/application/ops/metering"
	"github.com/agnivo/agnivo/packages/application/ops/notification"
	"github.com/agnivo/agnivo/packages/application/ops/quota"
	"github.com/agnivo/agnivo/packages/application/ops/store"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/jobs"
	"go.uber.org/zap"
)

// Module is the ops layer composition root.
type Module struct {
	Billing    *billing.Engine
	Metering   *metering.Collector
	Quota      *quota.Enforcer
	Notifier   *notification.Dispatcher
	Backup     *backup.Manager
	GC         *cleanup.GC
	Analytics  *analytics.Aggregator
	Audit      *audit.Logger
	AutoSleep  *autosleep.Manager
	Cron       *cron.Scheduler
	Publisher  *opsevents.Publisher
	Metrics    *opsmetrics.Metrics
}

// Init wires the complete Platform Operations Module.
// withCron = true when called from the cron executable;
// withWorker = true when called from the worker executable.
func Init(ctx context.Context, app *bootstrap.App, withWorker, withCron bool) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for ops module")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	cfg := app.Config.Ops

	// Prometheus metrics.
	m := opsmetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(m.Collectors()...)

	// Event bus.
	bus := events.NewInMemory(ctx, events.Config{Logger: app.Log})

	// Core repo and subsystems.
	repo := store.NewRepository(app.DB)
	pub := opsevents.NewPublisher(bus, app.Config.App.Name)

	// Billing provider (Nop by default; swap for a Stripe adapter in production).
	billingEngine := billing.NewEngine(repo, billing.NopProvider{}, app.Log)

	meteringCollector := metering.NewCollector(repo, app.Log)
	quotaEnforcer := quota.NewEnforcer(repo, app.Log)

	smtpCfg := notification.SMTPConfig{
		Host:     cfg.SMTPHost,
		Port:     cfg.SMTPPort,
		Username: cfg.SMTPUser,
		Password: cfg.SMTPPass,
		From:     cfg.SMTPFrom,
	}
	notifier := notification.NewDispatcher(repo, smtpCfg, app.Log)

	backupMgr := backup.NewManager(repo, nil, nil, cfg.BackupRetentionDays, app.Log)

	gcCfg := cleanup.DefaultConfig()
	gc := cleanup.NewGC(app.DB, gcCfg, app.Log)

	agg := analytics.NewAggregator(repo, app.DB, app.Log)
	auditLogger := audit.NewLogger(repo, app.Config.App.Name, app.Log)

	// Job queue for auto-sleep and billing work.
	jobMetrics := jobs.NewMetrics(app.Config.App.Name)
	app.Metrics.MustRegister(jobMetrics.Collectors()...)
	queue := jobs.NewQueue(app.DB, jobMetrics)

	sleepMgr := autosleep.NewManager(repo, queue, autosleep.DefaultThresholds(), app.Log)

	cronScheduler := cron.NewScheduler(repo, queue, app.Redis, idx.Prefixed("cron", 8), app.Log)

	mod := &Module{
		Billing:   billingEngine,
		Metering:  meteringCollector,
		Quota:     quotaEnforcer,
		Notifier:  notifier,
		Backup:    backupMgr,
		GC:        gc,
		Analytics: agg,
		Audit:     auditLogger,
		AutoSleep: sleepMgr,
		Cron:      cronScheduler,
		Publisher: pub,
		Metrics:   m,
	}

	if withWorker {
		registerWorkers(ctx, app, mod, queue, m, app.Log)
	}
	if withCron {
		registerCron(ctx, app, mod, queue, m, app.Log)
	}

	app.Log.Info("ops module initialized",
		zap.Bool("worker", withWorker),
		zap.Bool("cron", withCron))
	return mod, nil
}

// registerWorkers sets up all job-queue consumer workers.
func registerWorkers(_ context.Context, app *bootstrap.App, mod *Module, queue *jobs.Queue, m *opsmetrics.Metrics, log *zap.Logger) {
	handlers := opsjobs.NewHandlers(
		mod.Billing, mod.Metering, mod.Notifier, mod.Backup,
		mod.GC, mod.Analytics, mod.AutoSleep, mod.Publisher, m, log,
	)

	workerCfg := func(q string, concurrency int) jobs.WorkerConfig {
		return jobs.WorkerConfig{
			Queue:       q,
			Concurrency: concurrency,
			BatchSize:   concurrency * 2,
			PollInterval: time.Second,
			Visibility:  5 * time.Minute,
			Logger:      log,
		}
	}

	for _, spec := range []struct {
		name        string
		queue       string
		concurrency int
	}{
		{"ops-worker-ops", opsjobs.QueueOps, 8},
		{"ops-worker-notifications", opsjobs.QueueNotifications, 16},
		{"ops-worker-backup", opsjobs.QueueBackup, 2},
		{"ops-worker-cleanup", opsjobs.QueueCleanup, 4},
		{"ops-worker-analytics", opsjobs.QueueAnalytics, 2},
		{"ops-worker-billing", opsjobs.QueueBilling, 4},
		{"ops-worker-autosleep", autosleep.QueueAutosleep, 4},
	} {
		w := jobs.NewWorker(queue, workerCfg(spec.queue, spec.concurrency))
		handlers.Register(w)
		name := spec.name
		app.AddRunner(name, w.Run)
	}
}

// registerCron starts the distributed cron scheduler and seeds built-in jobs.
func registerCron(ctx context.Context, app *bootstrap.App, mod *Module, queue *jobs.Queue, _ *opsmetrics.Metrics, log *zap.Logger) {
	s := mod.Cron

	// Seed built-in schedules.
	builtin := []struct {
		name, schedule, q, jobType string
		payload                    any
	}{
		{"daily-usage-rollup", "@daily", opsjobs.QueueOps, opsjobs.TypeUsageRollup,
			map[string]string{"period": ""}},
		{"daily-analytics", "@daily", opsjobs.QueueAnalytics, opsjobs.TypeAnalytics,
			map[string]string{"period": ""}},
		{"hourly-notifications", "@hourly", opsjobs.QueueNotifications, opsjobs.TypeNotify,
			map[string]string{}},
		{"daily-backup", "@daily", opsjobs.QueueBackup, opsjobs.TypeBackupDB,
			map[string]string{"kind": "database"}},
		{"hourly-cleanup", "@hourly", opsjobs.QueueCleanup, opsjobs.TypeCleanup,
			map[string]string{"target": ""}},
		{"monthly-billing", "@monthly", opsjobs.QueueBilling, opsjobs.TypeBillingCycle,
			map[string]string{}},
	}
	for _, b := range builtin {
		if err := s.Register(ctx, b.name, b.schedule, "UTC", b.q, b.jobType, b.payload); err != nil {
			log.Warn("ops: register cron job failed",
				zap.String("name", b.name),
				zap.Error(err))
		}
	}

	app.AddRunner("ops-cron-scheduler", s.Run)
	_ = queue
	_ = app
}

// OpsConfig config subset (referenced via app.Config.Ops).
// Defined here to avoid a circular import with the platform/config package.
func opsConfig(cfg config.Config) config.OpsConfig {
	return cfg.Ops
}
