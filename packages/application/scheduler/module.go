package scheduler

import (
	"context"
	"time"

	schedhttp "github.com/agnivo/agnivo/packages/application/scheduler/http"
	"github.com/agnivo/agnivo/packages/application/scheduler/engine"
	schedevents "github.com/agnivo/agnivo/packages/application/scheduler/events"
	schedmetrics "github.com/agnivo/agnivo/packages/application/scheduler/metrics"
	"github.com/agnivo/agnivo/packages/application/scheduler/store"
	"github.com/agnivo/agnivo/packages/platform/bootstrap"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the scheduler composition root.
type Module struct {
	Engine  *engine.Engine
	HTTP    *schedhttp.Handlers
	Metrics *schedmetrics.Metrics
}

// Init wires the scheduler module.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for scheduler")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	bus := events.NewInMemory(ctx, events.Config{})
	app.AddHook(lifecycle.Hook{Name: "scheduler-event-bus", Stop: bus.Close})

	schedMetrics := schedmetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(schedMetrics.Collectors()...)

	repo := store.NewRepository(app.DB)
	eventRepo := schedevents.NewRepository(app.DB)
	publisher := schedevents.NewPublisher(bus, app.Config.App.Name, eventRepo)
	eng := engine.NewEngine(app.Config.Scheduler, repo, publisher, schedMetrics)
	handlers := schedhttp.NewHandlers(eng, repo, schedMetrics)

	if app.Config.Scheduler.InternalPort > 0 {
		app.RegisterInternalServer("scheduler-internal", app.Config.Scheduler.InternalPort, func(r chi.Router) {
			schedhttp.Mount(r, handlers)
		})
	}

	interval := app.Config.Scheduler.ReconcileInterval
	if interval <= 0 {
		interval = app.Config.Scheduler.HeartbeatInterval * 2
	}
	app.AddRunner("scheduler-reconcile", func(ctx context.Context) error {
		ticker := app.Config.Scheduler.ReconcileInterval
		if ticker <= 0 {
			ticker = interval
		}
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(ticker):
				if err := eng.Reconcile(ctx); err != nil {
					app.Log.Warn("scheduler reconcile failed", zap.Error(err))
				}
			}
		}
	})

	app.Log.Info("scheduler module initialized", zap.Int("port", app.Config.Scheduler.InternalPort))
	return &Module{Engine: eng, HTTP: handlers, Metrics: schedMetrics}, nil
}
