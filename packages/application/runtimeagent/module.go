package runtimeagent

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/docker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/executor"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/health"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/heartbeat"
	rthttp "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/http"
	rtmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/recovery"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	pevents "github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the runtime agent composition root.
type Module struct {
	Lifecycle *executor.Lifecycle
	HTTP      *rthttp.Handlers
	Metrics   *rtmetrics.Metrics
}

// Init wires the runtime agent module.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for runtime agent")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	bus := pevents.NewInMemory(ctx, pevents.Config{})
	app.AddHook(lifecycle.Hook{Name: "runtime-event-bus", Stop: bus.Close})

	rtMetrics := rtmetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(rtMetrics.Collectors()...)

	dockerClient, err := docker.NewClient(app.Config.RuntimeAgent)
	if err != nil {
		return nil, err
	}
	app.AddHook(lifecycle.Hook{Name: "runtime-docker", Stop: func(context.Context) error {
		return dockerClient.Close()
	}})

	if err := dockerClient.EnsureNetwork(ctx); err != nil {
		app.Log.Warn("runtime network setup", zap.Error(err))
	}
	if err := dockerClient.Ping(ctx); err != nil {
		app.Log.Warn("docker ping failed", zap.Error(err))
	}

	repo := store.NewRepository(app.DB)
	eventRepo := events.NewRepository(app.DB)
	publisher := events.NewPublisher(bus, app.Config.App.Name, eventRepo)
	lc := executor.NewLifecycle(app.Config.RuntimeAgent, dockerClient, repo, publisher, rtMetrics)
	handlers := rthttp.NewHandlers(lc, repo, health.NewMonitor(app.Config.RuntimeAgent, dockerClient, repo, publisher, rtMetrics))
	monitor := health.NewMonitor(app.Config.RuntimeAgent, dockerClient, repo, publisher, rtMetrics)
	reconciler := recovery.NewReconciler(app.Config.RuntimeAgent, dockerClient, repo, lc)
	hb := heartbeat.NewSender(app.Config.RuntimeAgent, dockerClient, repo, publisher)

	if app.Config.RuntimeAgent.InternalPort > 0 {
		app.RegisterInternalServer("runtime-internal", app.Config.RuntimeAgent.InternalPort, func(r chi.Router) {
			rthttp.Mount(r, handlers)
		})
	}

	app.AddRunner("runtime-heartbeat", hb.Run)
	app.AddRunner("runtime-health", monitor.Run)
	app.AddRunner("runtime-recovery", reconciler.Run)

	app.Log.Info("runtime agent initialized",
		zap.Int("port", app.Config.RuntimeAgent.InternalPort),
		zap.String("node_id", hb.NodeID()),
	)
	return &Module{Lifecycle: lc, HTTP: handlers, Metrics: rtMetrics}, nil
}
