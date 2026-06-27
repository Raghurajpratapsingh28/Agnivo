package http

import (
	"encoding/json"
	"net/http"

	"github.com/agnivo/agnivo/packages/application/scheduler/engine"
	"github.com/agnivo/agnivo/packages/application/scheduler/model"
	"github.com/agnivo/agnivo/packages/application/scheduler/store"
	schedmetrics "github.com/agnivo/agnivo/packages/application/scheduler/metrics"
	"github.com/agnivo/agnivo/packages/platform/dto"
	"github.com/go-chi/chi/v5"
)

// Handlers exposes internal scheduler HTTP endpoints.
type Handlers struct {
	engine  *engine.Engine
	store   *store.Repository
	metrics *schedmetrics.Metrics
}

// NewHandlers constructs internal HTTP handlers.
func NewHandlers(eng *engine.Engine, store *store.Repository, metrics *schedmetrics.Metrics) *Handlers {
	return &Handlers{engine: eng, store: store, metrics: metrics}
}

// Mount registers internal routes matching deployer client contract.
func Mount(r chi.Router, h *Handlers) {
	r.Route("/internal/v1", func(r chi.Router) {
		r.Post("/reserve", h.Reserve)
		r.Post("/release", h.Release)
		r.Post("/placement", h.Placement)
		r.Post("/heartbeat", h.Heartbeat)
		r.Get("/capacity", h.Capacity)
		r.Get("/servers", h.ListServers)
		r.Get("/health", h.Health)
		r.Get("/ready", h.Ready)
		r.Get("/live", h.Live)
		r.Get("/metrics/summary", h.MetricsSummary)
	})
}

type reserveRequest struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	DeploymentID  string `json:"deployment_id"`
	PortMin       int    `json:"port_min"`
	PortMax       int    `json:"port_max"`
	Region        string `json:"region"`
	Algorithm     string `json:"algorithm"`
	CorrelationID string `json:"correlation_id"`
	CPUMillicores int    `json:"cpu_millicores"`
	MemoryMB      int    `json:"memory_mb"`
}

type releaseRequest struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	DeploymentID  string `json:"deployment_id"`
	CorrelationID string `json:"correlation_id"`
}

func (h *Handlers) Reserve(w http.ResponseWriter, r *http.Request) {
	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		dto.Error(w, r, err)
		return
	}
	result, err := h.engine.Reserve(r.Context(), model.PlacementRequest{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		PortMin: req.PortMin, PortMax: req.PortMax, Region: req.Region,
		Algorithm: req.Algorithm, CorrelationID: req.CorrelationID,
		CPUMillicores: req.CPUMillicores, MemoryMB: req.MemoryMB,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, result)
}

func (h *Handlers) Release(w http.ResponseWriter, r *http.Request) {
	var req releaseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		dto.Error(w, r, err)
		return
	}
	if err := h.engine.Release(r.Context(), req.OrgID, req.ProjectID, req.DeploymentID, req.CorrelationID); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]any{"released": true})
}

func (h *Handlers) Placement(w http.ResponseWriter, r *http.Request) {
	var req reserveRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		dto.Error(w, r, err)
		return
	}
	result, err := h.engine.Simulate(r.Context(), model.PlacementRequest{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		PortMin: req.PortMin, PortMax: req.PortMax, Region: req.Region, Algorithm: req.Algorithm,
		CPUMillicores: req.CPUMillicores, MemoryMB: req.MemoryMB,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, result)
}

func (h *Handlers) Heartbeat(w http.ResponseWriter, r *http.Request) {
	var hb model.HeartbeatPayload
	if err := json.NewDecoder(r.Body).Decode(&hb); err != nil {
		dto.Error(w, r, err)
		return
	}
	srv, err := h.engine.RegisterHeartbeat(r.Context(), hb)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, srv)
}

func (h *Handlers) Capacity(w http.ResponseWriter, r *http.Request) {
	capacity, err := h.store.CapacitySummary(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, capacity)
}

func (h *Handlers) ListServers(w http.ResponseWriter, r *http.Request) {
	region := r.URL.Query().Get("region")
	servers, err := h.store.ListHealthyServers(r.Context(), region)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, servers)
}

func (h *Handlers) Health(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) Ready(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "ready"})
}

func (h *Handlers) Live(w http.ResponseWriter, _ *http.Request) {
	dto.OK(w, map[string]string{"status": "live"})
}

func (h *Handlers) MetricsSummary(w http.ResponseWriter, r *http.Request) {
	capacity, err := h.store.CapacitySummary(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, capacity)
}
