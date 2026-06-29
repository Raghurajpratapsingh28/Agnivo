package http

import (
	"net/http"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/buildstore"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/worker"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/dto"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/httpx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/go-chi/chi/v5"
)

// Handlers exposes internal builder HTTP endpoints.
type Handlers struct {
	builds  *buildstore.Repository
	logs    *buildstore.LogRepository
	queue   *jobs.Queue
	handler *worker.Handler
	cancels *cancel.Registry
}

// NewHandlers constructs internal HTTP handlers.
func NewHandlers(builds *buildstore.Repository, logs *buildstore.LogRepository, queue *jobs.Queue, handler *worker.Handler, cancels *cancel.Registry) *Handlers {
	return &Handlers{builds: builds, logs: logs, queue: queue, handler: handler, cancels: cancels}
}

// Mount registers internal routes on r.
func Mount(r chi.Router, h *Handlers) {
	r.Route("/internal/v1/builder", func(r chi.Router) {
		r.Get("/status", h.Status)
		r.Get("/builds/{deploymentID}", h.GetBuild)
		r.Get("/builds/{deploymentID}/logs", h.GetLogs)
		r.Post("/builds/{deploymentID}/cancel", h.CancelBuild)
		r.Get("/workers", h.WorkerStats)
		r.Get("/queue", h.QueueStats)
	})
}

// Status GET /internal/v1/builder/status
func (h *Handlers) Status(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]any{"service": "builder", "status": "ok"})
}

// GetBuild GET /internal/v1/builder/builds/{deploymentID}
func (h *Handlers) GetBuild(w http.ResponseWriter, r *http.Request) {
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	b, err := h.builds.GetByDeployment(r.Context(), depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, b)
}

// GetLogs GET /internal/v1/builder/builds/{deploymentID}/logs
func (h *Handlers) GetLogs(w http.ResponseWriter, r *http.Request) {
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	b, err := h.builds.GetByDeployment(r.Context(), depID)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	limit := 200
	if n, ok := httpx.QueryInt(r, "limit"); ok {
		limit = n
	}
	entries, err := h.logs.ListByBuild(r.Context(), b.ID, limit)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, entries)
}

// CancelBuild POST /internal/v1/builder/builds/{deploymentID}/cancel
func (h *Handlers) CancelBuild(w http.ResponseWriter, r *http.Request) {
	depID, err := httpx.RequirePathParam(r, "deploymentID")
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	ok := h.handler.CancelBuild(depID)
	dto.OK(w, map[string]any{"cancelled": ok, "deployment_id": depID})
}

// WorkerStats GET /internal/v1/builder/workers
func (h *Handlers) WorkerStats(w http.ResponseWriter, r *http.Request) {
	dto.OK(w, map[string]any{"active_builds": h.cancels.ActiveCount()})
}

// QueueStats GET /internal/v1/builder/queue
func (h *Handlers) QueueStats(w http.ResponseWriter, r *http.Request) {
	stats, err := h.queue.Stats(r.Context(), cpjobs.QueueBuilds)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, stats)
}
