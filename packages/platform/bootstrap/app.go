package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/cache/redis"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/health"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
	httpxmw "github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx/middleware"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/observability/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/observability/tracing"
	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// RegisterFunc is implemented by each executable to attach its routes, runners,
// hooks, and health checks. It is the only executable-specific code path.
type RegisterFunc func(ctx context.Context, app *App) error

// App is the running application handed to each executable's RegisterFunc. It
// exposes shared infrastructure and registration helpers but no business logic.
type App struct {
	Config  *config.Config
	Log     *zap.Logger
	Metrics *metrics.Registry
	Tracer  *tracing.Provider
	DB      *postgres.DB
	Redis   *redis.Client
	Health  *health.Registry

	// Router is the public HTTP router; nil when http.enabled is false.
	Router chi.Router

	lifecycle *lifecycle.Manager
}

// AddRunner registers a long-running component (e.g. a worker poll loop).
func (a *App) AddRunner(name string, run func(ctx context.Context) error) {
	a.lifecycle.AddRunner(lifecycle.Runner{Name: name, Run: run})
}

// AddHook registers a start/stop lifecycle hook.
func (a *App) AddHook(h lifecycle.Hook) { a.lifecycle.AddHook(h) }

// RegisterInternalServer mounts an authenticated internal HTTP API on port.
// All internal servers share recovery, correlation IDs, body limits, and optional
// bearer-token auth from security.internal_service_token.
func (a *App) RegisterInternalServer(name string, port int, mount func(chi.Router)) {
	if port <= 0 {
		return
	}
	addr := fmt.Sprintf("0.0.0.0:%d", port)
	if a.Config.Observability.AdminHost != "" {
		addr = fmt.Sprintf("%s:%d", a.Config.Observability.AdminHost, port)
	}
	r := chi.NewRouter()
	r.Use(httpxmw.RequestID)
	r.Use(httpxmw.CorrelationID)
	r.Use(httpxmw.Recovery)
	r.Use(httpxmw.Logger(a.Log))
	r.Use(httpxmw.MaxBodyBytes(a.Config.Security.MaxRequestBodyBytes))
	r.Use(httpxmw.InternalServiceAuth(*a.Config))
	mount(r)
	addServer(a, name, addr, r)
}

// RegisterHealth adds a readiness check.
func (a *App) RegisterHealth(name string, fn health.CheckFunc) { a.Health.Register(name, fn) }

// Run is the single entry point for every executable. It loads configuration,
// builds core infrastructure via Wire, wires health checks and the admin/public
// servers, invokes register, and runs the graceful lifecycle.
//
// Boot order:
//  1. Load configuration  2. Validate (in Load)  3. Logger  4. Metrics
//  5. Tracing  6. Database  7. Redis  8. Shared clients (DI/Wire)
//  9. Health checks  10. Admin server  11. register (routes/workers)
//  12. Public HTTP server  13. Run  14. Graceful shutdown
func Run(appName string, register RegisterFunc) error {
	ctx := context.Background()

	cfg, err := config.Load(config.Options{
		AppName:    appName,
		ConfigDir:  os.Getenv("AGNIVO_CONFIG_DIR"),
		DotEnvPath: ".env",
	})
	if err != nil {
		return err
	}

	core, err := initCore(ctx, cfg)
	if err != nil {
		return err
	}
	log := core.Logger
	defer func() { _ = log.Sync() }()

	app := &App{
		Config:    cfg,
		Log:       log,
		Metrics:   core.Metrics,
		Tracer:    core.Tracer,
		DB:        core.DB,
		Redis:     core.Redis,
		Health:    core.Health,
		lifecycle: lifecycle.New(log, cfg.App.ShutdownTimeout),
	}

	log.Info("starting",
		zap.Bool("http", cfg.HTTP.Enabled),
		zap.Bool("database", cfg.Database.Enabled),
		zap.Bool("redis", cfg.Redis.Enabled),
	)

	registerInfraHealth(app)
	registerInfraShutdown(app)
	registerInfraRunners(app)

	routerParams := httpx.RouterParams{Config: cfg, Logger: log, Metrics: core.Metrics, Health: core.Health}

	// Admin server (health + metrics) always runs on its own port.
	metricsHandler := promhttp.HandlerFor(core.Metrics.Prometheus(), promhttp.HandlerOpts{})
	adminRouter := httpx.NewAdminRouter(routerParams, metricsHandler)
	addServer(app, "admin", cfg.Observability.AdminAddr(), adminRouter)

	// Public router is built before register so executables can mount routes.
	if cfg.HTTP.Enabled {
		app.Router = httpx.NewRouter(routerParams)
	}

	if err := register(ctx, app); err != nil {
		return err
	}

	if cfg.HTTP.Enabled && app.Router != nil {
		addServer(app, "http", cfg.HTTP.Addr(), app.Router)
	}

	return app.lifecycle.Run(ctx)
}

// registerInfraHealth wires readiness checks for enabled dependencies.
func registerInfraHealth(app *App) {
	if app.DB != nil {
		app.Health.Register("postgres", app.DB.Check)
	}
	if app.Redis != nil {
		app.Health.Register("redis", app.Redis.Check)
	}
}

// registerInfraShutdown registers stop hooks so dependencies flush/close in the
// correct order during graceful shutdown.
func registerInfraShutdown(app *App) {
	if app.DB != nil {
		app.AddHook(lifecycle.Hook{Name: "postgres", Stop: func(context.Context) error {
			app.DB.Close()
			return nil
		}})
	}
	if app.Redis != nil {
		app.AddHook(lifecycle.Hook{Name: "redis", Stop: func(context.Context) error {
			return app.Redis.Close()
		}})
	}
	if app.Tracer != nil {
		app.AddHook(lifecycle.Hook{Name: "tracing", Stop: app.Tracer.Shutdown})
	}
}

// registerInfraRunners attaches background infrastructure loops (e.g. the
// database pool health monitor) to the lifecycle so they start and stop with
// the process.
func registerInfraRunners(app *App) {
	if app.DB != nil {
		app.AddRunner("db-monitor", app.DB.Monitor)
	}
}

// addServer registers an HTTP server as a runner plus a graceful-shutdown hook.
func addServer(app *App, name, addr string, handler http.Handler) {
	srv := &http.Server{
		Addr:              addr,
		Handler:           handler,
		ReadTimeout:       app.Config.HTTP.ReadTimeout,
		ReadHeaderTimeout: app.Config.HTTP.ReadHeaderTimeout,
		WriteTimeout:      app.Config.HTTP.WriteTimeout,
		IdleTimeout:       app.Config.HTTP.IdleTimeout,
	}

	app.AddRunner(name+"-server", func(ctx context.Context) error {
		app.Log.Info("http server listening", zap.String("server", name), zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			return err
		}
		return nil
	})
	app.AddHook(lifecycle.Hook{Name: name + "-server", Stop: func(ctx context.Context) error {
		return srv.Shutdown(ctx)
	}})
}
