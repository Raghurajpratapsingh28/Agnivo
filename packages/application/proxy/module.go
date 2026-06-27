// Package proxy is the Edge Networking composition root.
// It wires all subsystems — Caddy, DNS verification, certificate management,
// route engine, traffic switcher, preview manager, streaming hub, and the
// domain-job worker — into a single Module that proxy-manager registers.
package proxy

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/caddy"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/cert"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/dns"
	proxyevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/events"
	proxyhttp "github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/http"
	proxmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/preview"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/recovery"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/route"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/streaming"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/traffic"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/bootstrap"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/lifecycle"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// Module is the proxy-manager composition root.
type Module struct {
	Engine     *route.Engine
	Switcher   *traffic.Switcher
	Preview    *preview.Manager
	Cert       *cert.Manager
	Reconciler *recovery.Reconciler
	Hub        *streaming.Hub
	HTTP       *proxyhttp.Handlers
	Metrics    *proxmetrics.Metrics
	Publisher  *proxyevents.Publisher
}

// Init wires the complete edge networking module.
func Init(ctx context.Context, app *bootstrap.App) (*Module, error) {
	if app.DB == nil {
		return nil, errors.FailedPrecondition("database required for proxy-manager")
	}
	if err := app.DB.Migrate(ctx, Migrations()); err != nil {
		return nil, err
	}

	cfg := app.Config.ProxyManager

	// Prometheus metrics.
	proxyMetrics := proxmetrics.New(app.Config.App.Name)
	app.Metrics.MustRegister(proxyMetrics.Collectors()...)

	// In-process event bus.
	bus := events.NewInMemory(ctx, events.Config{Logger: app.Log})
	app.AddHook(lifecycle.Hook{Name: "proxy-event-bus", Stop: bus.Close})

	// Infrastructure.
	repo := store.NewRepository(app.DB)
	caddyClient := caddy.NewClient(cfg.CaddyAdminURL, app.Log)

	// Verify Caddy is reachable; warn but don't fail (Caddy may start after us).
	if err := caddyClient.Ping(ctx); err != nil {
		app.Log.Warn("proxy: caddy not reachable at startup",
			zap.String("url", cfg.CaddyAdminURL),
			zap.Error(err))
	}

	pub := proxyevents.NewPublisher(bus, repo, app.Config.App.Name)

	// Subsystems.
	dnsVerifier := dns.NewVerifier("agnivo-verify=", app.Log)
	routeEngine := route.NewEngine(repo, caddyClient, app.Log)
	trafficSwitcher := traffic.NewSwitcher(repo, caddyClient, app.Log)
	certManager := cert.NewManager(repo, caddyClient, cfg.CertRenewBefore, app.Log)

	previewTTL := cfg.PreviewTTL
	if previewTTL <= 0 {
		previewTTL = 7 * 24 * time.Hour
	}
	prevManager := preview.NewManager(repo, caddyClient, cfg.PreviewDomain, previewTTL, app.Log)

	// Streaming hub (requires Redis).
	var hub *streaming.Hub
	if app.Redis != nil {
		hub = streaming.NewHub(app.Redis, app.Log)
		app.AddRunner("proxy-streaming-hub", hub.Run)
		app.AddRunner("proxy-streaming-heartbeat", func(ctx context.Context) error {
			ticker := time.NewTicker(30 * time.Second)
			defer ticker.Stop()
			for {
				select {
				case <-ctx.Done():
					return ctx.Err()
				case <-ticker.C:
					hub.Heartbeat(ctx)
				}
			}
		})
	} else {
		hub = streaming.NewHub(nil, app.Log)
		app.Log.Warn("proxy: Redis not configured — streaming hub disabled")
	}

	// Reconciler drives background healing.
	reconciler := recovery.NewReconciler(
		repo, routeEngine, certManager, prevManager, dnsVerifier,
		pub, proxyMetrics, cfg, app.Log,
	)

	// HTTP handlers.
	handlers := proxyhttp.NewHandlers(
		repo, routeEngine, trafficSwitcher, prevManager,
		reconciler, hub, proxyMetrics, pub,
	)

	// Background reconciliation loop.
	app.AddRunner("proxy-reconciler", reconciler.Run)

	// Domain job worker (consumes domain.verify and domain.ssl_request jobs).
	if app.DB != nil {
		jobMetrics := jobs.NewMetrics(app.Config.App.Name)
		app.Metrics.MustRegister(jobMetrics.Collectors()...)
		queue := jobs.NewQueue(app.DB, jobMetrics)
		domainWorker := newDomainWorker(queue, reconciler, app.Log)
		app.AddRunner("proxy-domain-worker", domainWorker.Run)
	}

	// Internal HTTP server.
	if cfg.InternalPort > 0 {
		app.RegisterInternalServer("proxy-internal", cfg.InternalPort, func(r chi.Router) {
			proxyhttp.Mount(r, handlers)
		})
	}

	app.Log.Info("proxy-manager module initialized",
		zap.String("caddy_url", cfg.CaddyAdminURL),
		zap.String("platform_domain", cfg.PlatformDomain),
		zap.String("preview_domain", cfg.PreviewDomain),
		zap.Int("internal_port", cfg.InternalPort))

	return &Module{
		Engine:     routeEngine,
		Switcher:   trafficSwitcher,
		Preview:    prevManager,
		Cert:       certManager,
		Reconciler: reconciler,
		Hub:        hub,
		HTTP:       handlers,
		Metrics:    proxyMetrics,
		Publisher:  pub,
	}, nil
}

// ─────────────────────────────── Domain job worker ───────────────────────────

// domainWorker polls the domains queue and dispatches verify/ssl jobs to the
// reconciler so the proxy-manager does not depend on a separate worker binary.
type domainWorker struct {
	worker *jobs.Worker
}

func newDomainWorker(queue *jobs.Queue, reconciler *recovery.Reconciler, log *zap.Logger) *domainWorker {
	workerCfg := jobs.WorkerConfig{
		Queue:        cpjobs.QueueDomains,
		Concurrency:  4,
		BatchSize:    8,
		PollInterval: 2 * time.Second,
		Visibility:   5 * time.Minute,
		Logger:       log,
	}
	jw := jobs.NewWorker(queue, workerCfg)
	jw.Handle(cpjobs.TypeDomainVerify, func(ctx context.Context, j jobs.Job) error {
		var p cpjobs.Payload
		if err := j.Decode(&p); err != nil {
			return err
		}
		return reconciler.DomainVerifyRequest(ctx,
			p.OrgID, p.ProjectID, p.DomainID, "", "txt", p.CorrelationID)
	})
	jw.Handle(cpjobs.TypeSSLRequest, func(ctx context.Context, j jobs.Job) error {
		var p cpjobs.Payload
		if err := j.Decode(&p); err != nil {
			return err
		}
		return reconciler.SSLRequest(ctx,
			p.OrgID, p.ProjectID, p.DomainID, "", p.CorrelationID)
	})
	return &domainWorker{worker: jw}
}

func (dw *domainWorker) Run(ctx context.Context) error { return dw.worker.Run(ctx) }
