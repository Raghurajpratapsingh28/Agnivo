// Package http provides the proxy-manager internal HTTP API.
// All endpoints are admin/internal only — not reachable from the public internet.
package http

import (
	"net/http"

	proxyevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/events"
	proxmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/preview"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/recovery"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/route"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/streaming"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/traffic"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/go-chi/chi/v5"
)

// Handlers is the proxy-manager internal HTTP surface.
type Handlers struct {
	repo       *store.Repository
	engine     *route.Engine
	switcher   *traffic.Switcher
	prevMgr    *preview.Manager
	reconciler *recovery.Reconciler
	hub        *streaming.Hub
	metrics    *proxmetrics.Metrics
	pub        *proxyevents.Publisher
}

// NewHandlers constructs internal HTTP handlers.
func NewHandlers(
	repo *store.Repository,
	engine *route.Engine,
	switcher *traffic.Switcher,
	prevMgr *preview.Manager,
	reconciler *recovery.Reconciler,
	hub *streaming.Hub,
	metrics *proxmetrics.Metrics,
	pub *proxyevents.Publisher,
) *Handlers {
	return &Handlers{
		repo:       repo,
		engine:     engine,
		switcher:   switcher,
		prevMgr:    prevMgr,
		reconciler: reconciler,
		hub:        hub,
		metrics:    metrics,
		pub:        pub,
	}
}

// Mount registers all internal proxy routes.
func Mount(r chi.Router, h *Handlers) {
	r.Route("/internal/v1/proxy", func(r chi.Router) {
		// Health & status.
		r.Get("/health", h.Health)
		r.Get("/ready", h.Ready)
		r.Get("/live", h.Live)

		// Route management.
		r.Post("/routes", h.CreateRoute)
		r.Get("/routes/{hostname}", h.GetRoute)
		r.Delete("/routes/{hostname}", h.DeleteRoute)
		r.Get("/routes", h.ListRoutes)

		// Traffic switching.
		r.Post("/traffic/switch", h.Switch)
		r.Post("/traffic/blue-green", h.BlueGreen)
		r.Post("/traffic/canary", h.Canary)
		r.Post("/traffic/rollback", h.Rollback)

		// Preview environments.
		r.Post("/previews", h.CreatePreview)
		r.Get("/previews/{deployment_id}", h.GetPreview)
		r.Delete("/previews/{deployment_id}", h.DeletePreview)
		r.Get("/previews", h.ListPreviews)

		// Certificate & domain status.
		r.Get("/certs/{hostname}", h.GetCert)
		r.Post("/certs/{hostname}/renew", h.RenewCert)
		r.Get("/domains/{domain_id}/verification", h.GetVerification)

		// Metrics & statistics.
		r.Get("/stats", h.Stats)
		r.Get("/streaming/stats", h.StreamingStats)

		// On-demand domain jobs (consumed from the domains queue by the worker).
		r.Post("/jobs/domain-verify", h.JobDomainVerify)
		r.Post("/jobs/ssl-request", h.JobSSLRequest)
	})
}

// ─────────────────────────────── Health ─────────────────────────────────────

func (h *Handlers) Health(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) Ready(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "ready"})
}

func (h *Handlers) Live(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "live"})
}

// ─────────────────────────────── Routes ──────────────────────────────────────

type createRouteRequest struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	DeploymentID  string `json:"deployment_id"`
	DomainID      string `json:"domain_id"`
	Hostname      string `json:"hostname"`
	Upstream      string `json:"upstream"`
	Kind          string `json:"kind"`
	TLSEnabled    bool   `json:"tls_enabled"`
	HTTPSRedirect bool   `json:"https_redirect"`
	StripPrefix   string `json:"strip_prefix"`
	TimeoutSecs   int    `json:"timeout_seconds"`
	MaxRetries    int    `json:"max_retries"`
	CorrelationID string `json:"correlation_id"`
}

func (h *Handlers) CreateRoute(w http.ResponseWriter, r *http.Request) {
	var req createRouteRequest
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	rt, err := h.engine.Create(r.Context(), route.CreateInput{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		DomainID: req.DomainID, Hostname: req.Hostname, Upstream: req.Upstream,
		Kind: model.RouteKind(req.Kind), TLSEnabled: req.TLSEnabled,
		HTTPSRedirect: req.HTTPSRedirect, StripPrefix: req.StripPrefix,
		TimeoutSecs: req.TimeoutSecs, MaxRetries: req.MaxRetries,
		CorrelationID: req.CorrelationID,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, rt)
}

func (h *Handlers) GetRoute(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	rt, err := h.engine.Get(r.Context(), hostname)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, rt)
}

func (h *Handlers) DeleteRoute(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	if err := h.engine.Delete(r.Context(), hostname); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

func (h *Handlers) ListRoutes(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	projectID := r.URL.Query().Get("project_id")
	routes, err := h.engine.ListByProject(r.Context(), orgID, projectID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, routes)
}

// ─────────────────────────────── Traffic ─────────────────────────────────────

type switchRequest struct {
	Hostname         string `json:"hostname"`
	DeploymentID     string `json:"deployment_id"`
	NewUpstream      string `json:"new_upstream"`
	PreviousUpstream string `json:"previous_upstream"`
	Mode             string `json:"mode"`
	CanaryWeight     int    `json:"canary_weight"`
	CorrelationID    string `json:"correlation_id"`
}

func (h *Handlers) Switch(w http.ResponseWriter, r *http.Request) {
	var req switchRequest
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	rt, err := h.switcher.Switch(r.Context(), model.TrafficSwitchRequest{
		Hostname:         req.Hostname,
		DeploymentID:     req.DeploymentID,
		NewUpstream:      req.NewUpstream,
		PreviousUpstream: req.PreviousUpstream,
		Mode:             model.TrafficMode(req.Mode),
		CanaryWeight:     req.CanaryWeight,
		CorrelationID:    req.CorrelationID,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, rt)
}

func (h *Handlers) BlueGreen(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hostname      string `json:"hostname"`
		NewUpstream   string `json:"new_upstream"`
		CorrelationID string `json:"correlation_id"`
	}
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	rt, err := h.switcher.BlueGreen(r.Context(), req.Hostname, req.NewUpstream, req.CorrelationID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, rt)
}

func (h *Handlers) Canary(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hostname       string `json:"hostname"`
		CanaryUpstream string `json:"canary_upstream"`
		Weight         int    `json:"weight"`
		CorrelationID  string `json:"correlation_id"`
	}
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	rt, err := h.switcher.Canary(r.Context(), req.Hostname, req.CanaryUpstream, req.Weight, req.CorrelationID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, rt)
}

func (h *Handlers) Rollback(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Hostname      string `json:"hostname"`
		CorrelationID string `json:"correlation_id"`
	}
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	rt, err := h.switcher.Rollback(r.Context(), req.Hostname, req.CorrelationID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, rt)
}

// ─────────────────────────────── Previews ────────────────────────────────────

type createPreviewRequest struct {
	OrgID          string `json:"org_id"`
	ProjectID      string `json:"project_id"`
	DeploymentID   string `json:"deployment_id"`
	Upstream       string `json:"upstream"`
	Branch         string `json:"branch"`
	CommitSHA      string `json:"commit_sha"`
	CustomHostname string `json:"custom_hostname"`
	TTLSeconds     int    `json:"ttl_seconds"`
	CorrelationID  string `json:"correlation_id"`
}

func (h *Handlers) CreatePreview(w http.ResponseWriter, r *http.Request) {
	var req createPreviewRequest
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	p, err := h.prevMgr.Create(r.Context(), preview.CreateInput{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		Upstream: req.Upstream, Branch: req.Branch, CommitSHA: req.CommitSHA,
		CustomHostname: req.CustomHostname, CorrelationID: req.CorrelationID,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.Created(w, p)
}

func (h *Handlers) GetPreview(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deployment_id")
	p, err := h.prevMgr.Get(r.Context(), deploymentID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, p)
}

func (h *Handlers) DeletePreview(w http.ResponseWriter, r *http.Request) {
	deploymentID := chi.URLParam(r, "deployment_id")
	if err := h.prevMgr.Delete(r.Context(), deploymentID); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.NoContent(w)
}

func (h *Handlers) ListPreviews(w http.ResponseWriter, r *http.Request) {
	orgID := r.URL.Query().Get("org_id")
	projectID := r.URL.Query().Get("project_id")
	list, err := h.prevMgr.List(r.Context(), orgID, projectID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, list)
}

// ─────────────────────────────── Certs & Domains ─────────────────────────────

func (h *Handlers) GetCert(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	c, err := h.repo.GetCertByHostname(r.Context(), hostname)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, c)
}

func (h *Handlers) RenewCert(w http.ResponseWriter, r *http.Request) {
	hostname := chi.URLParam(r, "hostname")
	c, err := h.repo.GetCertByHostname(r.Context(), hostname)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]any{"hostname": hostname, "status": c.Status, "renew_after": c.RenewAfter})
}

func (h *Handlers) GetVerification(w http.ResponseWriter, r *http.Request) {
	domainID := chi.URLParam(r, "domain_id")
	v, err := h.repo.GetVerificationByDomain(r.Context(), domainID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, v)
}

// ─────────────────────────────── Metrics & Stats ─────────────────────────────

func (h *Handlers) Stats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.repo.Stats(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, stats)
}

func (h *Handlers) StreamingStats(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, h.hub.Stats())
}

// ─────────────────────────────── Job triggers ────────────────────────────────

type domainJobRequest struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	DomainID      string `json:"domain_id"`
	Hostname      string `json:"hostname"`
	Method        string `json:"method"`
	CorrelationID string `json:"correlation_id"`
}

func (h *Handlers) JobDomainVerify(w http.ResponseWriter, r *http.Request) {
	var req domainJobRequest
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	if err := h.reconciler.DomainVerifyRequest(r.Context(),
		req.OrgID, req.ProjectID, req.DomainID, req.Hostname, req.Method, req.CorrelationID,
	); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]bool{"queued": true})
}

func (h *Handlers) JobSSLRequest(w http.ResponseWriter, r *http.Request) {
	var req domainJobRequest
	if err := dto.Decode(w, r, &req); err != nil {
		dto.Error(w, r, err)
		return
	}
	if err := h.reconciler.SSLRequest(r.Context(),
		req.OrgID, req.ProjectID, req.DomainID, req.Hostname, req.CorrelationID,
	); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]bool{"queued": true})
}
