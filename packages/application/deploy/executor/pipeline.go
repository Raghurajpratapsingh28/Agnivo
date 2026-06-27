package executor

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	buildmodel "github.com/agnivo/agnivo/packages/application/build/buildstore"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpevents"
	"github.com/agnivo/agnivo/packages/application/controlplane/cpjobs"
	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	"github.com/agnivo/agnivo/packages/application/controlplane/project"
	deploycancel "github.com/agnivo/agnivo/packages/application/deploy/cancel"
	"github.com/agnivo/agnivo/packages/application/deploy/deploystore"
	deployecr "github.com/agnivo/agnivo/packages/application/deploy/ecr"
	deployevents "github.com/agnivo/agnivo/packages/application/deploy/events"
	"github.com/agnivo/agnivo/packages/application/deploy/health"
	deploymetrics "github.com/agnivo/agnivo/packages/application/deploy/metrics"
	deploymodel "github.com/agnivo/agnivo/packages/application/deploy/model"
	"github.com/agnivo/agnivo/packages/application/deploy/rollback"
	"github.com/agnivo/agnivo/packages/application/deploy/runtime"
	"github.com/agnivo/agnivo/packages/application/deploy/scheduler"
	"github.com/agnivo/agnivo/packages/application/deploy/secrets"
	"github.com/agnivo/agnivo/packages/application/deploy/strategy"
	"github.com/agnivo/agnivo/packages/platform/config"
	"github.com/agnivo/agnivo/packages/platform/errors"
	"github.com/agnivo/agnivo/packages/platform/logger"
)

// Pipeline orchestrates deployment execution.
type Pipeline struct {
	cfg          config.Deployer
	deployments  *deployment.Repository
	projects     *project.Repository
	artifacts    *buildmodel.ArtifactRepository
	executions   *deploystore.Repository
	containers   *deploystore.ContainerRepository
	healthRepo   *deploystore.HealthRepository
	events       *deployevents.Publisher
	cpEvents     *cpevents.Publisher
	secrets      *secrets.Loader
	puller       *deployecr.Puller
	scheduler    *scheduler.Client
	runtime      runtime.Driver
	health       *health.Checker
	strategies   *strategy.Registry
	rollback     *rollback.Engine
	metrics      *deploymetrics.Metrics
	cancels      *deploycancel.Registry
	workerID     string
}

// Deps wires pipeline dependencies.
type Deps struct {
	Config       config.Deployer
	Deployments  *deployment.Repository
	Projects     *project.Repository
	Artifacts    *buildmodel.ArtifactRepository
	Executions   *deploystore.Repository
	Containers   *deploystore.ContainerRepository
	HealthRepo   *deploystore.HealthRepository
	Events       *deployevents.Publisher
	CPEvents     *cpevents.Publisher
	Secrets      *secrets.Loader
	Puller       *deployecr.Puller
	Scheduler    *scheduler.Client
	Runtime      runtime.Driver
	Health       *health.Checker
	Strategies   *strategy.Registry
	Rollback     *rollback.Engine
	Metrics      *deploymetrics.Metrics
	Cancels      *deploycancel.Registry
	WorkerID     string
}

// NewPipeline constructs a deployment pipeline.
func NewPipeline(d Deps) *Pipeline {
	return &Pipeline{
		cfg: d.Config, deployments: d.Deployments, projects: d.Projects, artifacts: d.Artifacts,
		executions: d.Executions, containers: d.Containers, healthRepo: d.HealthRepo,
		events: d.Events, cpEvents: d.CPEvents, secrets: d.Secrets, puller: d.Puller,
		scheduler: d.Scheduler, runtime: d.Runtime, health: d.Health, strategies: d.Strategies,
		rollback: d.Rollback, metrics: d.Metrics, cancels: d.Cancels, workerID: d.WorkerID,
	}
}

// RunDeploy executes a deploy.run job.
func (p *Pipeline) RunDeploy(ctx context.Context, payload cpjobs.Payload, jobID string) error {
	return p.run(ctx, payload, jobID, false)
}

// RunRollback executes a deploy.rollback job.
func (p *Pipeline) RunRollback(ctx context.Context, payload cpjobs.Payload, jobID string) error {
	return p.run(ctx, payload, jobID, true)
}

func (p *Pipeline) run(ctx context.Context, payload cpjobs.Payload, jobID string, isRollback bool) error {
	start := time.Now()
	p.metrics.IncActive()
	defer p.metrics.DecActive()

	ctx, cancelFn := context.WithCancel(ctx)
	defer cancelFn()
	p.cancels.Register(payload.DeploymentID, cancelFn)
	defer p.cancels.Unregister(payload.DeploymentID)

	if payload.DeploymentID == "" {
		return errors.New(errors.CodeInvalidArgument, "deploy: missing deployment_id")
	}

	dep, err := p.deployments.GetByID(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}

	if cancelled, _ := p.deployments.IsCancelled(ctx, payload.OrgID, payload.DeploymentID); cancelled {
		return p.handleCancel(ctx, payload, "deployment cancelled")
	}

	// Wait for build to complete unless rollback with image_tag already set
	ready, err := p.deployments.IsReadyToDeploy(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}
	if !ready && !isRollback {
		// Retry later — build still in progress
		return errors.New(errors.CodeFailedPrecond, "deploy: waiting for build to complete")
	}

	proj, err := p.projects.GetByID(ctx, payload.OrgID, payload.ProjectID)
	if err != nil {
		return err
	}

	artifact, err := p.artifacts.GetByDeployment(ctx, payload.DeploymentID)
	if err != nil && dep.ImageTag == "" {
		return errors.Wrap(err, errors.CodeFailedPrecond, "deploy: artifact not found")
	}

	imageRef := firstNonEmpty(dep.ImageTag, artifact.ImageTag)
	digest := artifact.ImageDigest
	buildID := artifact.BuildID
	var buildIDPtr *string
	if buildID != "" {
		buildIDPtr = &buildID
	}

	strategyName := strategy.Resolve(dep.Environment, p.cfg.DefaultStrategy, dep.Environment == "preview", isRollback)
	jobIDPtr := &jobID
	exec, err := p.executions.UpsertForDeployment(ctx, deploymodel.Execution{
		OrgID: payload.OrgID, ProjectID: payload.ProjectID, DeploymentID: payload.DeploymentID,
		JobID: jobIDPtr, Status: deploymodel.ExecQueued, Phase: deploymodel.PhaseQueued,
		Strategy: strategyName, Environment: dep.Environment, ImageTag: imageRef,
		ImageDigest: digest, BuildID: buildIDPtr, CorrelationID: payload.CorrelationID,
	})
	if err != nil {
		return err
	}

	meta := p.eventMeta(exec, payload)
	_ = p.events.Publish(ctx, deployevents.DeploymentQueued, meta, nil)
	_ = p.executions.MarkRunning(ctx, exec.ID, p.workerID)
	_ = p.events.Publish(ctx, deployevents.DeploymentStarted, meta, nil)

	if err := p.transition(ctx, exec, deploymodel.PhaseScheduling, deployment.StatusScheduling, payload, "scheduling resources"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}

	reserveStart := time.Now()
	placement, err := p.scheduler.Reserve(ctx, payload.OrgID, payload.ProjectID, payload.DeploymentID,
		p.cfg.Network.HostPortMin, p.cfg.Network.HostPortMax)
	if err != nil {
		return p.fail(ctx, payload, exec, meta, start, "scheduler reserve failed")
	}
	p.metrics.ObserveReservation(time.Since(reserveStart).Seconds())
	defer func() { _ = p.scheduler.Release(ctx, payload.OrgID, payload.ProjectID, payload.DeploymentID) }()

	if err := p.checkCancel(ctx, payload); err != nil {
		return err
	}
	if err := p.transition(ctx, exec, deploymodel.PhaseResourcesReserved, deployment.StatusScheduling, payload, "resources reserved"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}
	_ = p.events.Publish(ctx, deployevents.ResourcesReserved, meta, map[string]any{"host": placement.Host, "port": placement.Port})

	// Pull image
	if err := p.transition(ctx, exec, deploymodel.PhasePullingImage, deployment.StatusDeploying, payload, "pulling image"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}
	pullRes, err := p.puller.Pull(ctx, deployecr.PullOptions{
		ImageRef: imageRef, ExpectedDigest: digest,
	})
	if err != nil {
		return p.fail(ctx, payload, exec, meta, start, "image pull failed")
	}
	p.metrics.ObservePull(pullRes.Duration.Seconds())
	_ = p.events.Publish(ctx, deployevents.ImagePulled, meta, map[string]any{"digest": pullRes.Digest, "cached": pullRes.Cached})

	// Load secrets/env
	if err := p.transition(ctx, exec, deploymodel.PhasePreparingRuntime, deployment.StatusDeploying, payload, "preparing runtime"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}
	runtimeCfg, err := p.secrets.LoadRuntimeConfig(ctx, payload.OrgID, payload.ProjectID, dep.Environment)
	if err != nil {
		return p.fail(ctx, payload, exec, meta, start, "configuration load failed")
	}
	runtimeCfg.Image = imageRef
	runtimeCfg.Port = p.cfg.Health.TCPPort
	if runtimeCfg.Port <= 0 {
		runtimeCfg.Port = 8080
	}
	runtimeCfg.HostPort = placement.Port
	runtimeCfg.Labels["agnivo.deployment_id"] = payload.DeploymentID
	runtimeCfg.Labels["agnivo.project_slug"] = proj.Slug
	if placement.AgentURL != "" {
		if runtimeCfg.Annotations == nil {
			runtimeCfg.Annotations = map[string]string{}
		}
		runtimeCfg.Annotations["agnivo.agent_url"] = placement.AgentURL
	}

	if err := p.transition(ctx, exec, deploymodel.PhaseCreatingContainer, deployment.StatusDeploying, payload, "creating container"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}

	sctx := strategy.Context{
		DeploymentID: payload.DeploymentID, Environment: dep.Environment,
		Strategy: strategyName, Runtime: runtimeCfg,
		Placement: strategy.Placement{Host: placement.Host, Port: placement.Port},
		IsPreview: dep.Environment == "preview", IsRollback: isRollback,
	}
	stratResult, err := p.strategies.Get(strategyName).Deploy(ctx, sctx, p.runtime)
	if err != nil {
		return p.fail(ctx, payload, exec, meta, start, "container creation failed")
	}
	_ = p.events.Publish(ctx, deployevents.ContainerCreated, meta, map[string]string{"container_id": stratResult.Container.ID})
	_ = p.containers.Create(ctx, deploymodel.ContainerRecord{
		ExecutionID: exec.ID, DeploymentID: payload.DeploymentID,
		ContainerID: stratResult.Container.ID, Image: imageRef,
		Host: placement.Host, Port: placement.Port, Status: "running", Role: "active",
	})

	if err := p.transition(ctx, exec, deploymodel.PhaseWaitingHealth, deployment.StatusDeploying, payload, "waiting for health"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}

	healthStart := time.Now()
	checks, err := p.health.WaitUntilHealthy(ctx, placement.Host, placement.Port)
	for _, c := range checks {
		_ = p.healthRepo.Record(ctx, deploymodel.HealthRecord{
			ExecutionID: exec.ID, DeploymentID: payload.DeploymentID,
			CheckType: c.CheckType, Success: c.Success, LatencyMs: c.Latency.Milliseconds(), Message: c.Message,
		})
		ev := deployevents.HealthCheckPassed
		if !c.Success {
			ev = deployevents.HealthCheckFailed
		}
		_ = p.events.Publish(ctx, ev, meta, map[string]any{"type": c.CheckType, "latency_ms": c.Latency.Milliseconds()})
	}
	p.metrics.ObserveHealth(time.Since(healthStart).Seconds())
	if err != nil {
		if p.rollback != nil && rollback.ShouldRollback(rollback.TriggerHealthFailed) {
			_, _ = p.rollback.RestorePrevious(ctx, payload.OrgID, payload.ProjectID, payload.DeploymentID, meta)
		}
		return p.fail(ctx, payload, exec, meta, start, "health check failed")
	}

	if err := p.transition(ctx, exec, deploymodel.PhaseSwitchingTraffic, deployment.StatusDeploying, payload, "switching traffic"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}
	_ = p.events.Publish(ctx, deployevents.TrafficSwitched, meta, map[string]string{"host": placement.Host})

	// Drain previous containers
	if err := p.transition(ctx, exec, deploymodel.PhaseCleanup, deployment.StatusDeploying, payload, "cleanup"); err != nil {
		return p.fail(ctx, payload, exec, meta, start, err.Error())
	}
	oldContainers, _ := p.containers.ListActiveByProject(ctx, payload.OrgID, payload.ProjectID)
	var drainIDs []string
	for _, c := range oldContainers {
		if c.ContainerID != stratResult.Container.ID && c.DeploymentID != payload.DeploymentID {
			drainIDs = append(drainIDs, c.ContainerID)
		}
	}
	if len(drainIDs) > 0 {
		drained := p.rollback.DrainContainers(ctx, drainIDs)
		for _, id := range drained {
			_ = p.containers.MarkRemoved(ctx, id)
			_ = p.events.Publish(ctx, deployevents.ContainerRemoved, meta, map[string]string{"container_id": id})
		}
	}

	totalMs := time.Since(start).Milliseconds()
	deployMeta, _ := json.Marshal(map[string]any{
		"strategy": strategyName, "container_id": stratResult.Container.ID,
		"host": placement.Host, "port": placement.Port, "deployer_version": p.cfg.Version,
	})

	if isRollback {
		_, err = p.deployments.MarkRolledBack(ctx, payload.OrgID, payload.DeploymentID, totalMs)
	} else {
		_, err = p.deployments.UpdateDeployComplete(ctx, payload.OrgID, payload.DeploymentID, deployment.DeployResult{
			ImageTag: imageRef, ImageDigest: digest, DeployDurationMs: totalMs,
			Strategy: strategyName, ContainerID: stratResult.Container.ID, Metadata: deployMeta,
		})
	}
	if err != nil {
		return err
	}

	_ = p.executions.MarkSucceeded(ctx, exec.ID, stratResult.Container.ID, placement.Host, placement.Port, totalMs)
	_ = p.transition(ctx, exec, deploymodel.PhaseComplete, deployment.StatusLive, payload, "deployment complete")
	_ = p.events.Publish(ctx, deployevents.DeploymentSucceeded, meta, map[string]any{"duration_ms": totalMs})
	if p.cpEvents != nil {
		_ = p.cpEvents.PublishAsync(ctx, cpevents.DeploymentSucceeded, cpevents.Meta{
			OrgID: payload.OrgID, ProjectID: payload.ProjectID, AggregateID: payload.DeploymentID,
			CorrelationID: payload.CorrelationID,
		}, map[string]any{"container_id": stratResult.Container.ID})
	}
	p.metrics.IncSuccess()
	p.metrics.ObserveDeploy(strategyName, "success", time.Since(start).Seconds())
	return nil
}

// RunCancel handles deploy.delete — stop containers and mark cancelled.
func (p *Pipeline) RunCancel(ctx context.Context, payload cpjobs.Payload) error {
	containers, _ := p.containers.ListActiveByDeployment(ctx, payload.DeploymentID)
	for _, c := range containers {
		_ = p.runtime.Stop(ctx, c.ContainerID, 10*time.Second)
		_ = p.runtime.Remove(ctx, c.ContainerID)
		_ = p.containers.MarkRemoved(ctx, c.ContainerID)
	}
	exec, _ := p.executions.GetByDeployment(ctx, payload.DeploymentID)
	if exec.ID != "" {
		_ = p.executions.MarkCancelled(ctx, exec.ID)
	}
	return p.handleCancel(ctx, payload, "deployment cancelled")
}

// RunSleep stops all active containers for a project.
func (p *Pipeline) RunSleep(ctx context.Context, payload cpjobs.Payload) error {
	containers, err := p.containers.ListActiveByProject(ctx, payload.OrgID, payload.ProjectID)
	if err != nil {
		return err
	}
	for _, c := range containers {
		_ = p.runtime.Stop(ctx, c.ContainerID, 30*time.Second)
		_ = p.containers.MarkStopped(ctx, c.ContainerID)
	}
	return nil
}

// RunWake restarts sleeping containers or re-enqueues deploy for latest.
func (p *Pipeline) RunWake(ctx context.Context, payload cpjobs.Payload) error {
	if payload.DeploymentID == "" {
		return nil
	}
	dep, err := p.deployments.GetByID(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}
	payload.CorrelationID = dep.CorrelationID
	return p.RunDeploy(ctx, payload, "")
}

func (p *Pipeline) transition(ctx context.Context, exec deploymodel.Execution, phase deploymodel.Phase, cpStatus deployment.Status, payload cpjobs.Payload, message string) error {
	from := exec.Phase
	if err := p.executions.SetPhase(ctx, exec.ID, payload.DeploymentID, from, phase, message); err != nil {
		return err
	}
	_, err := p.deployments.UpdateStatus(ctx, payload.OrgID, payload.DeploymentID, cpStatus, message, "")
	return err
}

func (p *Pipeline) fail(ctx context.Context, payload cpjobs.Payload, exec deploymodel.Execution, meta deployevents.Meta, start time.Time, reason string) error {
	totalMs := time.Since(start).Milliseconds()
	_, _ = p.deployments.MarkDeployFailed(ctx, payload.OrgID, payload.DeploymentID, reason, totalMs)
	_ = p.executions.MarkFailed(ctx, exec.ID, reason, totalMs)
	_ = p.executions.SetPhase(ctx, exec.ID, payload.DeploymentID, exec.Phase, deploymodel.PhaseFailed, reason)
	_ = p.events.Publish(ctx, deployevents.DeploymentFailed, meta, map[string]string{"reason": reason})
	if p.cpEvents != nil {
		_ = p.cpEvents.PublishAsync(ctx, cpevents.DeploymentFailed, cpevents.Meta{
			OrgID: payload.OrgID, ProjectID: payload.ProjectID, AggregateID: payload.DeploymentID,
			CorrelationID: payload.CorrelationID,
		}, map[string]string{"reason": reason})
	}
	p.metrics.IncFailure()
	p.metrics.ObserveDeploy(exec.Strategy, "failed", time.Since(start).Seconds())
	return errors.New(errors.CodeFailedPrecond, reason)
}

func (p *Pipeline) handleCancel(ctx context.Context, payload cpjobs.Payload, msg string) error {
	exec, _ := p.executions.GetByDeployment(ctx, payload.DeploymentID)
	if exec.ID != "" {
		_ = p.executions.MarkCancelled(ctx, exec.ID)
		_ = p.events.Publish(ctx, deployevents.DeploymentCancelled, p.eventMeta(exec, payload), map[string]string{"reason": msg})
	}
	return errors.New(errors.CodeCanceled, msg)
}

func (p *Pipeline) checkCancel(ctx context.Context, payload cpjobs.Payload) error {
	cancelled, err := p.deployments.IsCancelled(ctx, payload.OrgID, payload.DeploymentID)
	if err != nil {
		return err
	}
	if cancelled || ctx.Err() != nil {
		return p.handleCancel(ctx, payload, "deployment cancelled")
	}
	return nil
}

func (p *Pipeline) eventMeta(exec deploymodel.Execution, payload cpjobs.Payload) deployevents.Meta {
	return deployevents.Meta{
		EventID: "", ExecutionID: exec.ID, DeploymentID: payload.DeploymentID,
		OrgID: payload.OrgID, ProjectID: payload.ProjectID, CorrelationID: payload.CorrelationID,
	}
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// WithCorrelation attaches correlation ID to context.
func WithCorrelation(ctx context.Context, correlationID string) context.Context {
	if correlationID == "" {
		return ctx
	}
	return logger.WithCorrelationID(ctx, correlationID)
}
