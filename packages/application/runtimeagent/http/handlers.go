package http

import (
	"encoding/json"
	"net/http"
	"strconv"
	"time"

	"github.com/agnivo/agnivo/packages/application/runtimeagent/executor"
	"github.com/agnivo/agnivo/packages/application/runtimeagent/health"
	"github.com/agnivo/agnivo/packages/application/runtimeagent/model"
	"github.com/agnivo/agnivo/packages/application/runtimeagent/store"
	"github.com/agnivo/agnivo/packages/platform/dto"
	"github.com/agnivo/agnivo/packages/platform/httpx"
	"github.com/go-chi/chi/v5"
)

// Handlers exposes internal runtime agent HTTP endpoints.
type Handlers struct {
	lifecycle *executor.Lifecycle
	store     *store.Repository
	health    *health.Monitor
}

// NewHandlers constructs internal HTTP handlers.
func NewHandlers(lifecycle *executor.Lifecycle, store *store.Repository, monitor *health.Monitor) *Handlers {
	return &Handlers{lifecycle: lifecycle, store: store, health: monitor}
}

// Mount registers internal routes consumed by the deployer AgentDriver.
func Mount(r chi.Router, h *Handlers) {
	r.Route("/internal/v1/runtime", func(r chi.Router) {
		r.Get("/health", h.Health)
		r.Get("/ready", h.Ready)
		r.Get("/live", h.Live)
		r.Get("/containers", h.ListContainers)
		r.Post("/containers", h.CreateContainer)
		r.Get("/containers/{containerID}", h.GetContainer)
		r.Post("/containers/{containerID}/start", h.StartContainer)
		r.Post("/containers/{containerID}/stop", h.StopContainer)
		r.Post("/containers/{containerID}/restart", h.RestartContainer)
		r.Delete("/containers/{containerID}", h.DeleteContainer)
		r.Get("/containers/{containerID}/health", h.ContainerHealth)
		r.Get("/containers/{containerID}/logs", h.ContainerLogs)
		r.Get("/diagnostics", h.Diagnostics)
	})
}

type createBody struct {
	DeploymentID  string            `json:"deployment_id"`
	Image         string            `json:"image"`
	Env           map[string]string `json:"env"`
	Secrets       map[string]string `json:"secrets"`
	Labels        map[string]string `json:"labels"`
	Port          int               `json:"port"`
	HostPort      int               `json:"host_port"`
	Network       string            `json:"network"`
	CorrelationID string            `json:"correlation_id"`
}

func (h *Handlers) CreateContainer(w http.ResponseWriter, r *http.Request) {
	var body createBody
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		dto.Error(w, r, err)
		return
	}
	info, err := h.lifecycle.Create(r.Context(), model.CreateRequest{
		DeploymentID: body.DeploymentID, Image: body.Image, Env: body.Env, Secrets: body.Secrets,
		Labels: body.Labels, Port: body.Port, HostPort: body.HostPort, Network: body.Network,
		CorrelationID: body.CorrelationID,
	})
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, info)
}

func (h *Handlers) StartContainer(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	corr := r.URL.Query().Get("correlation_id")
	if err := h.lifecycle.Start(r.Context(), id, corr); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]string{"status": "started"})
}

func (h *Handlers) StopContainer(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	secs := 10
	if v := r.URL.Query().Get("timeout"); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			secs = n
		}
	}
	if err := h.lifecycle.Stop(r.Context(), id, time.Duration(secs)*time.Second, r.URL.Query().Get("correlation_id")); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]string{"status": "stopped"})
}

func (h *Handlers) RestartContainer(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	if err := h.lifecycle.Restart(r.Context(), id, r.URL.Query().Get("correlation_id")); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]string{"status": "restarted"})
}

func (h *Handlers) DeleteContainer(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	if err := h.lifecycle.Remove(r.Context(), id, r.URL.Query().Get("correlation_id")); err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, map[string]string{"status": "deleted"})
}

func (h *Handlers) GetContainer(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	info, err := h.lifecycle.Inspect(r.Context(), id)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, info)
}

func (h *Handlers) ListContainers(w http.ResponseWriter, r *http.Request) {
	list, err := h.store.ListActive(r.Context())
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, list)
}

func (h *Handlers) ContainerHealth(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	report, err := h.health.Report(r.Context(), id)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, report)
}

func (h *Handlers) ContainerLogs(w http.ResponseWriter, r *http.Request) {
	id, _ := httpx.RequirePathParam(r, "containerID")
	limit := 100
	if n, ok := httpx.QueryInt(r, "limit"); ok {
		limit = n
	}
	logs, err := h.store.ListLogs(r.Context(), id, limit)
	if err != nil {
		dto.Error(w, r, err)
		return
	}
	dto.OK(w, logs)
}

func (h *Handlers) Diagnostics(w http.ResponseWriter, r *http.Request) {
	count, _ := h.store.CountActive(r.Context())
	dto.OK(w, map[string]any{"active_containers": count, "status": "ok"})
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
