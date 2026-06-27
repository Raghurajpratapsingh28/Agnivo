package http

import (
	"net/http"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	deploycancel "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/deploystore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/worker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/go-chi/chi/v5"
)

// Handlers exposes internal deployer HTTP endpoints.
type Handlers struct {
	executions *deploystore.Repository
	health     *deploystore.HealthRepository
	queue      *jobs.Queue
	handler    *worker.Handler
	cancels    *deploycancel.Registry
}

// NewHandlers constructs internal HTTP handlers.
func NewHandlers(executions *deploystore.Repository, health *deploystore.HealthRepository, queue *jobs.Queue, handler *worker.Handler, cancels *deploycancel.Registry) *Handlers {
	return &Handlers{executions: executions, health: health, queue: queue, handler: handler, cancels: cancels}
}

// Mount registers internal routes.
func Mount(r chi.Router, h *Handlers) {
	r.Route("/internal/v1/deployer", func(r chi.Router) {
		r.Get("/health", h.Health)
		r.Get("/ready", h.Ready)
		r.Get("/live", h.Live)
		r.Get("/status", h.Status)
		r.Get("/deployments/{deploymentID}", h.GetDeployment)
		r.Get("/deployments/{deploymentID}/phases", h.GetPhases)
		r.Get("/deployments/{deploymentID}/health", h.GetHealth)
		r.Post("/deployments/{deploymentID}/cancel", h.CancelDeployment)
		r.Get("/metrics/summary", h.MetricsSummary)
		r.Get("/workers", h.WorkerStats)
		r.Get("/queue", h.QueueStats)
	})
}

func (h *Handlers) Health(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]string{"status": "ok"})
}

func (h *Handlers) Ready(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]string{"status": "ready"})
}

func (h *Handlers) Live(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]string{"status": "live"})
}

func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]any{"service": "deployer", "status": "ok"})
}

func (h *Handlers) GetDeployment(w http.ResponseWriter, r *http.Request) {
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	exec, err := h.executions.GetByDeployment(r.Context(), depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, exec)
}

func (h *Handlers) GetPhases(w http.ResponseWriter, r *http.Request) {
	depID, _ := httpx.RequirePathParam(r, "deploymentID")
	phases, err := h.executions.ListPhaseTransitions(r.Context(), depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, phases)
}

func (h *Handlers) GetHealth(w http.ResponseWriter, r *http.Request) {
	depID, _ := httpx.RequirePathParam(r, "deploymentID")
	limit := 50
	if n, ok := httpx.QueryInt(r, "limit"); ok {
		limit = n
	}
	records, err := h.health.ListByDeployment(r.Context(), depID, limit)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, records)
}

func (h *Handlers) CancelDeployment(w http.ResponseWriter, r *http.Request) {
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ok := h.handler.CancelDeployment(depID)
	dto.OK(w, map[string]any{"cancelled": ok, "deployment_id": depID})
}

func (h *Handlers) MetricsSummary(w http.ResponseWriter, r *http.Request) {
	stats, err := h.executions.Stats(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, stats)
}

func (h *Handlers) WorkerStats(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]any{"active_deployments": h.cancels.ActiveCount()})
}

func (h *Handlers) QueueStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queue.Stats(r.Context(), cpjobs.QueueDeployments)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, stats)
}
