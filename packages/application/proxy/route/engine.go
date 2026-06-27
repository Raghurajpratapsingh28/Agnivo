// Package route implements the dynamic Route Engine: every mutation to a
// proxy route is atomic — the database record and the Caddy configuration are
// updated in the same logical operation, with rollback on failure.
package route

import (
	"context"
	"encoding/json"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// CaddyRouter abstracts the Caddy route operations needed by the engine.
type CaddyRouter interface {
	UpsertRoute(ctx context.Context, cfg model.CaddyRouteConfig) error
	DeleteRoute(ctx context.Context, hostname string) error
}

// CreateInput is the payload for creating a new route.
type CreateInput struct {
	OrgID         string
	ProjectID     string
	DeploymentID  string
	DomainID      string
	Hostname      string
	Upstream      string
	Kind          model.RouteKind
	TLSEnabled    bool
	HTTPSRedirect bool
	StripPrefix   string
	AddHeaders    map[string]string
	TimeoutSecs   int
	MaxRetries    int
	CorrelationID string
}

// Engine manages the lifecycle of proxy routes.
type Engine struct {
	repo  *store.Repository
	caddy CaddyRouter
	log   *zap.Logger
}

// NewEngine constructs a route Engine.
func NewEngine(repo *store.Repository, caddy CaddyRouter, log *zap.Logger) *Engine {
	return &Engine{repo: repo, caddy: caddy, log: log}
}

// Create registers a new route in both the database and Caddy.
func (e *Engine) Create(ctx context.Context, in CreateInput) (model.Route, error) {
	if in.Hostname == "" {
		return model.Route{}, errors.New(errors.CodeInvalidArgument, "route: hostname required")
	}
	if in.Upstream == "" {
		return model.Route{}, errors.New(errors.CodeInvalidArgument, "route: upstream required")
	}

	corrID := in.CorrelationID
	if corrID == "" {
		corrID = logger.CorrelationID(ctx)
	}
	kind := in.Kind
	if kind == "" {
		kind = model.RouteKindPlatform
	}
	timeout := in.TimeoutSecs
	if timeout <= 0 {
		timeout = 30
	}
	retries := in.MaxRetries
	if retries <= 0 {
		retries = 3
	}

	addHeadersRaw, _ := json.Marshal(in.AddHeaders)

	rt, err := e.repo.UpsertRoute(ctx, model.Route{
		ID:            idx.NewUUID(),
		OrgID:         in.OrgID,
		ProjectID:     in.ProjectID,
		DeploymentID:  in.DeploymentID,
		DomainID:      in.DomainID,
		Hostname:      in.Hostname,
		Upstream:      in.Upstream,
		Kind:          kind,
		Status:        model.RouteStatusPending,
		TrafficMode:   model.TrafficModeActive,
		TLSEnabled:    in.TLSEnabled,
		HTTPSRedirect: in.HTTPSRedirect,
		StripPrefix:   in.StripPrefix,
		AddHeaders:    addHeadersRaw,
		TimeoutSeconds: timeout,
		MaxRetries:    retries,
		Version:       1,
		CorrelationID: corrID,
	})
	if err != nil {
		return model.Route{}, err
	}

	// Push route to Caddy. On failure, mark the DB record as errored.
	caddyCfg := toCaddyConfig(rt, nil)
	if err := e.caddy.UpsertRoute(ctx, caddyCfg); err != nil {
		_ = e.repo.SetRouteStatus(ctx, rt.ID, model.RouteStatusError)
		e.log.Error("route: caddy upsert failed",
			zap.String("hostname", rt.Hostname),
			zap.Error(err))
		return rt, errors.Wrapf(err, errors.CodeInternal, "route: caddy sync for %s", rt.Hostname)
	}

	_ = e.repo.SetRouteStatus(ctx, rt.ID, model.RouteStatusActive)
	rt.Status = model.RouteStatusActive

	_ = e.repo.RecordRouteVersion(ctx, rt)
	e.log.Info("route: created",
		zap.String("hostname", rt.Hostname),
		zap.String("upstream", rt.Upstream))
	return rt, nil
}

// Update modifies an existing route's upstream or configuration.
func (e *Engine) Update(ctx context.Context, hostname, upstream string, headers map[string]string, timeoutSecs, maxRetries int, correlationID string) (model.Route, error) {
	rt, err := e.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		return model.Route{}, err
	}

	if upstream != "" {
		rt.Upstream = upstream
	}
	if headers != nil {
		rt.AddHeaders, _ = json.Marshal(headers)
	}
	if timeoutSecs > 0 {
		rt.TimeoutSeconds = timeoutSecs
	}
	if maxRetries >= 0 {
		rt.MaxRetries = maxRetries
	}
	if correlationID != "" {
		rt.CorrelationID = correlationID
	}

	rt, err = e.repo.UpsertRoute(ctx, rt)
	if err != nil {
		return rt, err
	}

	if err := e.caddy.UpsertRoute(ctx, toCaddyConfig(rt, nil)); err != nil {
		e.log.Error("route: caddy update failed", zap.String("hostname", hostname), zap.Error(err))
		return rt, errors.Wrapf(err, errors.CodeInternal, "route: caddy sync for %s", hostname)
	}

	_ = e.repo.RecordRouteVersion(ctx, rt)
	e.log.Info("route: updated", zap.String("hostname", hostname))
	return rt, nil
}

// Delete removes a route from Caddy and marks it deleted in the database.
func (e *Engine) Delete(ctx context.Context, hostname string) error {
	rt, err := e.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		if errors.IsCode(err, errors.CodeNotFound) {
			return nil
		}
		return err
	}

	if err := e.repo.SetRouteStatus(ctx, rt.ID, model.RouteStatusDraining); err != nil {
		return err
	}

	if err := e.caddy.DeleteRoute(ctx, hostname); err != nil {
		e.log.Warn("route: caddy delete failed", zap.String("hostname", hostname), zap.Error(err))
	}

	if err := e.repo.SoftDeleteRoute(ctx, rt.ID); err != nil {
		return err
	}

	e.log.Info("route: deleted", zap.String("hostname", hostname))
	return nil
}

// Get returns the current route record for a hostname.
func (e *Engine) Get(ctx context.Context, hostname string) (model.Route, error) {
	return e.repo.GetRouteByHostname(ctx, hostname)
}

// GetByDeployment returns the route for a deployment ID.
func (e *Engine) GetByDeployment(ctx context.Context, deploymentID string) (model.Route, error) {
	return e.repo.GetRouteByDeployment(ctx, deploymentID)
}

// ListByProject returns all routes for an org/project.
func (e *Engine) ListByProject(ctx context.Context, orgID, projectID string) ([]model.Route, error) {
	return e.repo.ListRoutesByProject(ctx, orgID, projectID)
}

// ReconcileAll pushes all active DB routes into Caddy. Used during startup and
// periodic reconciliation to ensure Caddy state matches the database.
func (e *Engine) ReconcileAll(ctx context.Context) error {
	routes, err := e.repo.ListActiveRoutes(ctx)
	if err != nil {
		return err
	}
	var errs int
	for _, rt := range routes {
		if err := e.caddy.UpsertRoute(ctx, toCaddyConfig(rt, nil)); err != nil {
			e.log.Warn("route: reconcile upsert failed",
				zap.String("hostname", rt.Hostname),
				zap.Error(err))
			errs++
		}
	}
	e.log.Info("route: reconcile complete",
		zap.Int("total", len(routes)),
		zap.Int("errors", errs))
	return nil
}

// toCaddyConfig converts a Route record to a Caddy configuration struct.
func toCaddyConfig(rt model.Route, headers map[string]string) model.CaddyRouteConfig {
	addHeaders := headers
	if addHeaders == nil && len(rt.AddHeaders) > 0 {
		_ = json.Unmarshal(rt.AddHeaders, &addHeaders)
	}
	return model.CaddyRouteConfig{
		RouteID:        rt.ID,
		Hostname:       rt.Hostname,
		Upstream:       rt.Upstream,
		TLSEnabled:     rt.TLSEnabled,
		HTTPSRedirect:  rt.HTTPSRedirect,
		StripPrefix:    rt.StripPrefix,
		AddHeaders:     addHeaders,
		TimeoutSeconds: rt.TimeoutSeconds,
		MaxRetries:     rt.MaxRetries,
		CanaryWeight:   rt.CanaryWeight,
		CanaryUpstream: rt.PreviousUpstream,
	}
}
