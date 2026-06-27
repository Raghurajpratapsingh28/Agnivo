package engine

import (
	"context"
	"time"

	schedevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/events"
	schedmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/placement"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/scheduler/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Engine orchestrates placement and reservations.
type Engine struct {
	cfg     config.Scheduler
	store   *store.Repository
	placers *placement.Registry
	events  *schedevents.Publisher
	metrics *schedmetrics.Metrics
}

// NewEngine constructs a scheduling engine.
func NewEngine(cfg config.Scheduler, store *store.Repository, events *schedevents.Publisher, metrics *schedmetrics.Metrics) *Engine {
	return &Engine{
		cfg: cfg, store: store,
		placers: placement.NewRegistry(cfg.DefaultAlgorithm),
		events:  events, metrics: metrics,
	}
}

// RegisterHeartbeat processes runtime agent heartbeat.
func (e *Engine) RegisterHeartbeat(ctx context.Context, hb model.HeartbeatPayload) (model.Server, error) {
	srv, err := e.store.UpsertServer(ctx, hb)
	if err != nil {
		return model.Server{}, err
	}
	_ = e.events.Publish(ctx, schedevents.HeartbeatReceived, schedevents.Meta{
		ServerID: srv.ID, NodeID: srv.NodeID, CorrelationID: hb.NodeID,
	}, map[string]any{"container_count": hb.ContainerCount})
	if hb.HealthStatus == model.HealthHealthy && srv.MissedBeats > 0 {
		_ = e.events.Publish(ctx, schedevents.ServerRecovered, schedevents.Meta{
			ServerID: srv.ID, NodeID: srv.NodeID,
		}, nil)
	}
	return srv, nil
}

// Reserve places a deployment and holds resources.
func (e *Engine) Reserve(ctx context.Context, req model.PlacementRequest) (model.PlacementResult, error) {
	start := time.Now()
	defer func() {
		if e.metrics != nil {
			e.metrics.ObservePlacement(time.Since(start).Seconds())
		}
	}()

	if req.DeploymentID == "" {
		return model.PlacementResult{}, errors.New(errors.CodeInvalidArgument, "scheduler: missing deployment_id")
	}

	// Idempotent: return existing reservation
	if existing, err := e.store.GetReservation(ctx, req.DeploymentID); err == nil && existing.Status == model.ReservationActive {
		return model.PlacementResult{
			Host: existing.Host, Port: existing.Port, NodeID: existing.NodeID,
			Reserved: true,
		}, nil
	}

	_ = e.events.Publish(ctx, schedevents.PlacementRequested, schedevents.Meta{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		CorrelationID: req.CorrelationID,
	}, map[string]any{"algorithm": req.Algorithm})

	if req.CPUMillicores <= 0 {
		req.CPUMillicores = e.cfg.DefaultCPUMillicores
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = e.cfg.DefaultMemoryMB
	}
	algo := req.Algorithm
	if algo == "" {
		algo = e.cfg.DefaultAlgorithm
	}

	servers, err := e.store.ListHealthyServers(ctx, req.Region)
	if err != nil {
		return model.PlacementResult{}, err
	}
	if len(servers) == 0 {
		_ = e.events.Publish(ctx, schedevents.PlacementFailed, schedevents.Meta{
			OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		}, map[string]string{"reason": "no healthy servers"})
		if e.metrics != nil {
			e.metrics.IncPlacementFailure()
		}
		return model.PlacementResult{}, errors.New(errors.CodeUnavailable, "scheduler: no capacity")
	}

	placer := e.placers.Get(algo)
	srv, port, ok := placer.Select(servers, req, e.cfg.OvercommitRatio)
	if !ok {
		_ = e.events.Publish(ctx, schedevents.PlacementFailed, schedevents.Meta{
			OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		}, map[string]string{"reason": "no fit"})
		if e.metrics != nil {
			e.metrics.IncPlacementFailure()
		}
		return model.PlacementResult{}, errors.New(errors.CodeFailedPrecond, "scheduler: placement failed")
	}

	host := srv.AdvertiseHost
	if host == "" {
		host = srv.Hostname
	}
	if host == "" {
		host = "127.0.0.1"
	}

	ttl := e.cfg.ReservationTTL
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	_, err = e.store.CreateReservation(ctx, model.Reservation{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		ServerID: srv.ID, NodeID: srv.NodeID, Host: host, Port: port,
		CPUMillicores: req.CPUMillicores, MemoryMB: req.MemoryMB,
		Algorithm: algo, Status: model.ReservationActive,
		ExpiresAt: time.Now().Add(ttl), CorrelationID: req.CorrelationID,
	})
	if err != nil {
		return model.PlacementResult{}, err
	}

	result := model.PlacementResult{
		Host: host, Port: port, NodeID: srv.NodeID, Region: srv.Region,
		AgentURL: srv.AgentURL, Reserved: true,
	}
	_ = e.events.Publish(ctx, schedevents.ReservationCreated, schedevents.Meta{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		ServerID: srv.ID, NodeID: srv.NodeID, CorrelationID: req.CorrelationID,
	}, map[string]any{"host": host, "port": port})
	_ = e.events.Publish(ctx, schedevents.PlacementSucceeded, schedevents.Meta{
		OrgID: req.OrgID, ProjectID: req.ProjectID, DeploymentID: req.DeploymentID,
		ServerID: srv.ID, CorrelationID: req.CorrelationID,
	}, map[string]any{"algorithm": algo})
	if e.metrics != nil {
		e.metrics.IncPlacementSuccess()
	}
	return result, nil
}

// Release frees a reservation.
func (e *Engine) Release(ctx context.Context, orgID, projectID, deploymentID, correlationID string) error {
	if err := e.store.ReleaseReservation(ctx, orgID, projectID, deploymentID); err != nil {
		return err
	}
	_ = e.events.Publish(ctx, schedevents.ReservationReleased, schedevents.Meta{
		OrgID: orgID, ProjectID: projectID, DeploymentID: deploymentID, CorrelationID: correlationID,
	}, nil)
	return nil
}

// Simulate runs placement without persisting.
func (e *Engine) Simulate(ctx context.Context, req model.PlacementRequest) (model.PlacementResult, error) {
	servers, err := e.store.ListHealthyServers(ctx, req.Region)
	if err != nil {
		return model.PlacementResult{}, err
	}
	if req.CPUMillicores <= 0 {
		req.CPUMillicores = e.cfg.DefaultCPUMillicores
	}
	if req.MemoryMB <= 0 {
		req.MemoryMB = e.cfg.DefaultMemoryMB
	}
	algo := req.Algorithm
	if algo == "" {
		algo = e.cfg.DefaultAlgorithm
	}
	srv, port, ok := e.placers.Get(algo).Select(servers, req, e.cfg.OvercommitRatio)
	if !ok {
		return model.PlacementResult{}, errors.New(errors.CodeFailedPrecond, "scheduler: simulation failed")
	}
	host := srv.AdvertiseHost
	if host == "" {
		host = srv.Hostname
	}
	return model.PlacementResult{
		Host: host, Port: port, NodeID: srv.NodeID, Region: srv.Region,
		AgentURL: srv.AgentURL, Reserved: false,
	}, nil
}

// Reconcile expires stale reservations and checks heartbeats.
func (e *Engine) Reconcile(ctx context.Context) error {
	if _, err := e.store.ExpireStaleReservations(ctx); err != nil {
		return err
	}
	threshold := e.cfg.HeartbeatInterval * time.Duration(e.cfg.MissedHeartbeats)
	if threshold <= 0 {
		threshold = 45 * time.Second
	}
	if _, err := e.store.MarkMissedHeartbeats(ctx, threshold, e.cfg.MissedHeartbeats); err != nil {
		return err
	}
	capacity, err := e.store.CapacitySummary(ctx)
	if err != nil {
		return err
	}
	_ = e.events.Publish(ctx, schedevents.CapacityUpdated, schedevents.Meta{}, capacity)
	return nil
}
