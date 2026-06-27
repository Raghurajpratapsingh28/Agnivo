package httpx

import (
	"net/http"

	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/health"
	mw "github.com/agnivo/agnivo/packages/platform/httpx/middleware"
	"github.com/agnivo/agnivo/packages/platform/observability/metrics"
	"github.com/go-chi/chi/v5"
	"go.uber.org/zap"
)

// RouterParams are the dependencies required to build the base router.
type RouterParams struct {
	Config  *config.Config
	Logger  *zap.Logger
	Metrics *metrics.Registry
	Health  *health.Registry
}

// NewRouter builds the base public router with the standard middleware chain and
// mounted health endpoints. Feature routes are mounted by each executable.
//
// Middleware order is deliberate: IDs first (so everything downstream is
// correlated), then recovery (so panics in later middleware are caught), then
// logging, security, CORS, compression, metrics, and finally the per-request
// timeout closest to handlers.
func NewRouter(p RouterParams) chi.Router {
	r := chi.NewRouter()

	r.Use(mw.RequestID)
	r.Use(mw.CorrelationID)
	r.Use(mw.Recovery)
	r.Use(mw.Logger(p.Logger))
	r.Use(mw.SecurityHeaders(p.Config.App.Environment.IsProduction()))
	r.Use(mw.MaxBodyBytes(p.Config.Security.MaxRequestBodyBytes))
	r.Use(mw.CORS(p.Config.HTTP.CORS))
	r.Use(mw.Compression())
	if p.Metrics != nil {
		r.Use(p.Metrics.Middleware)
	}
	r.Use(mw.Timeout(p.Config.HTTP.RequestTimeout))

	mountHealth(r, p.Health)
	return r
}

// NewAdminRouter builds the admin router serving health and metrics. It runs on
// a separate port so liveness/readiness and scraping never compete with traffic.
// When MetricsBearerToken is configured, /metrics requires Authorization: Bearer.
func NewAdminRouter(p RouterParams, metricsHandler http.Handler) chi.Router {
	r := chi.NewRouter()
	r.Use(mw.RequestID)
	r.Use(mw.Recovery)
	mountHealth(r, p.Health)
	if p.Config != nil && p.Config.Security.MetricsBearerToken != "" {
		r.Group(func(r chi.Router) {
			r.Use(mw.BearerToken(p.Config.Security.MetricsBearerToken))
			r.Method(http.MethodGet, "/metrics", metricsHandler)
		})
	} else {
		r.Method(http.MethodGet, "/metrics", metricsHandler)
	}
	return r
}

func mountHealth(r chi.Router, h *health.Registry) {
	if h == nil {
		return
	}
	r.Get("/health/live", h.Live)
	r.Get("/health/ready", h.Ready)
}
