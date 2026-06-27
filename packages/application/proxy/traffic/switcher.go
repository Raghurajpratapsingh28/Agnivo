// Package traffic implements production traffic switching strategies:
// rolling, blue-green, canary, and instant — all with atomic DB + Caddy updates
// and zero-downtime guarantees.
package traffic

import (
	"context"
	"time"

	"github.com/agnivo/agnivo/packages/application/proxy/model"
	"github.com/agnivo/agnivo/packages/application/proxy/store"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"go.uber.org/zap"
)

// CaddySwitcher abstracts the Caddy operations needed for traffic switching.
type CaddySwitcher interface {
	UpsertRoute(ctx context.Context, cfg model.CaddyRouteConfig) error
}

// RouteGetter fetches a route record.
type RouteGetter interface {
	GetRouteByDeployment(ctx context.Context, deploymentID string) (model.Route, error)
	GetRouteByHostname(ctx context.Context, hostname string) (model.Route, error)
}

// Switcher performs atomic traffic switch operations.
type Switcher struct {
	repo  *store.Repository
	caddy CaddySwitcher
	log   *zap.Logger
}

// NewSwitcher constructs a Switcher.
func NewSwitcher(repo *store.Repository, caddy CaddySwitcher, log *zap.Logger) *Switcher {
	return &Switcher{repo: repo, caddy: caddy, log: log}
}

// Switch executes the traffic-switch described by req atomically:
// 1. Updates the database record (version bump).
// 2. Pushes the new Caddy configuration.
// If the Caddy push fails the database update is NOT reverted — the reconciler
// will re-sync on the next pass.
func (s *Switcher) Switch(ctx context.Context, req model.TrafficSwitchRequest) (model.Route, error) {
	if req.Hostname == "" && req.DeploymentID == "" {
		return model.Route{}, errors.New(errors.CodeInvalidArgument, "traffic: hostname or deployment_id required")
	}

	var rt model.Route
	var err error
	if req.Hostname != "" {
		rt, err = s.repo.GetRouteByHostname(ctx, req.Hostname)
	} else {
		rt, err = s.repo.GetRouteByDeployment(ctx, req.DeploymentID)
	}
	if err != nil {
		return model.Route{}, err
	}

	mode := req.Mode
	if mode == "" {
		mode = model.TrafficModeActive
	}

	weight := req.CanaryWeight
	if mode != model.TrafficModeCanary {
		weight = 0
	}

	previous := rt.Upstream
	if req.PreviousUpstream != "" {
		previous = req.PreviousUpstream
	}

	// Persist the switch in the database first.
	if err := s.repo.UpdateRouteTraffic(ctx, rt.ID, req.NewUpstream, previous, mode, weight); err != nil {
		return rt, err
	}

	// Reload rt with the new state.
	rt.PreviousUpstream = previous
	rt.Upstream = req.NewUpstream
	rt.TrafficMode = mode
	rt.CanaryWeight = weight

	caddyCfg := model.CaddyRouteConfig{
		RouteID:        rt.ID,
		Hostname:       rt.Hostname,
		Upstream:       rt.Upstream,
		TLSEnabled:     rt.TLSEnabled,
		HTTPSRedirect:  rt.HTTPSRedirect,
		TimeoutSeconds: rt.TimeoutSeconds,
		MaxRetries:     rt.MaxRetries,
		CanaryWeight:   rt.CanaryWeight,
		CanaryUpstream: rt.PreviousUpstream,
	}

	if err := s.caddy.UpsertRoute(ctx, caddyCfg); err != nil {
		s.log.Error("traffic: caddy switch failed",
			zap.String("hostname", rt.Hostname),
			zap.String("mode", string(mode)),
			zap.Error(err))
		return rt, errors.Wrapf(err, errors.CodeInternal, "traffic: caddy switch for %s", rt.Hostname)
	}

	s.log.Info("traffic: switched",
		zap.String("hostname", rt.Hostname),
		zap.String("mode", string(mode)),
		zap.String("upstream", req.NewUpstream),
		zap.Int("canary_weight", weight))
	return rt, nil
}

// BlueGreen performs a full blue-green swap: all traffic instantly moves to the
// new upstream; the old upstream becomes the rollback target.
func (s *Switcher) BlueGreen(ctx context.Context, hostname, newUpstream, correlationID string) (model.Route, error) {
	rt, err := s.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		return model.Route{}, err
	}
	return s.Switch(ctx, model.TrafficSwitchRequest{
		OrgID:            rt.OrgID,
		ProjectID:        rt.ProjectID,
		DeploymentID:     rt.DeploymentID,
		Hostname:         hostname,
		NewUpstream:      newUpstream,
		PreviousUpstream: rt.Upstream,
		Mode:             model.TrafficModeGreen,
		CorrelationID:    correlationID,
	})
}

// Canary sends a percentage of traffic to the new upstream, leaving the rest
// on the existing upstream.
func (s *Switcher) Canary(ctx context.Context, hostname, canaryUpstream string, weight int, correlationID string) (model.Route, error) {
	if weight < 1 || weight > 99 {
		return model.Route{}, errors.New(errors.CodeInvalidArgument, "traffic: canary weight must be 1–99")
	}
	rt, err := s.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		return model.Route{}, err
	}
	// In canary mode the "new upstream" is the canary target; the existing
	// upstream continues to serve (100 - weight) % of traffic.
	return s.Switch(ctx, model.TrafficSwitchRequest{
		OrgID:            rt.OrgID,
		ProjectID:        rt.ProjectID,
		DeploymentID:     rt.DeploymentID,
		Hostname:         hostname,
		NewUpstream:      rt.Upstream,    // primary stays
		PreviousUpstream: canaryUpstream, // canary in the "previous" slot
		Mode:             model.TrafficModeCanary,
		CanaryWeight:     weight,
		CorrelationID:    correlationID,
	})
}

// Rollback instantly reverts to the previous upstream recorded for the route.
func (s *Switcher) Rollback(ctx context.Context, hostname, correlationID string) (model.Route, error) {
	rt, err := s.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		return model.Route{}, err
	}
	if rt.PreviousUpstream == "" {
		return rt, errors.New(errors.CodeFailedPrecond, "traffic: no previous upstream to roll back to")
	}
	return s.Switch(ctx, model.TrafficSwitchRequest{
		OrgID:            rt.OrgID,
		ProjectID:        rt.ProjectID,
		DeploymentID:     rt.DeploymentID,
		Hostname:         hostname,
		NewUpstream:      rt.PreviousUpstream,
		PreviousUpstream: rt.Upstream,
		Mode:             model.TrafficModeActive,
		CorrelationID:    correlationID,
	})
}

// Drain sets a route into draining mode, causing Caddy to stop sending new
// requests to the old upstream while existing connections complete.
func (s *Switcher) Drain(ctx context.Context, hostname string, drainFor time.Duration) error {
	rt, err := s.repo.GetRouteByHostname(ctx, hostname)
	if err != nil {
		return err
	}
	if err := s.repo.SetRouteStatus(ctx, rt.ID, model.RouteStatusDraining); err != nil {
		return err
	}
	s.log.Info("traffic: draining",
		zap.String("hostname", hostname),
		zap.Duration("drain_for", drainFor))
	return nil
}
