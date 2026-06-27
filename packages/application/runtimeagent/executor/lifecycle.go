package executor

import (
	"context"
	"time"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/docker"
	rtevents "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/events"
	rtmetrics "github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/metrics"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/runtimeagent/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/config"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
)

// Lifecycle manages container lifecycle operations.
type Lifecycle struct {
	cfg     config.RuntimeAgent
	docker  *docker.Client
	store   *store.Repository
	events  *rtevents.Publisher
	metrics *rtmetrics.Metrics
}

// NewLifecycle constructs a lifecycle manager.
func NewLifecycle(cfg config.RuntimeAgent, docker *docker.Client, store *store.Repository, events *rtevents.Publisher, metrics *rtmetrics.Metrics) *Lifecycle {
	return &Lifecycle{cfg: cfg, docker: docker, store: store, events: events, metrics: metrics}
}

// Create pulls image, creates container, persists state.
func (l *Lifecycle) Create(ctx context.Context, req model.CreateRequest) (model.ContainerInfo, error) {
	start := time.Now()
	if err := l.docker.EnsureNetwork(ctx); err != nil {
		return model.ContainerInfo{}, err
	}
	pullStart := time.Now()
	if err := l.docker.PullImage(ctx, req.Image); err != nil {
		return model.ContainerInfo{}, err
	}
	if l.metrics != nil {
		l.metrics.ObservePull(time.Since(pullStart).Seconds())
	}
	_ = l.events.Publish(ctx, rtevents.ImagePulled, rtevents.Meta{
		DeploymentID: req.DeploymentID, CorrelationID: req.CorrelationID,
	}, map[string]string{"image": req.Image})

	info, err := l.docker.CreateContainer(ctx, req)
	if err != nil {
		return model.ContainerInfo{}, err
	}
	_ = l.store.UpsertContainer(ctx, model.ContainerRecord{
		ContainerID: info.ID, DeploymentID: req.DeploymentID, Name: info.Name,
		Image: req.Image, Status: model.StatusCreated, HostPort: req.HostPort,
		ContainerPort: req.Port, CorrelationID: req.CorrelationID,
	})
	_ = l.store.SetStatus(ctx, info.ID, "", model.StatusCreated, "container created")
	_ = l.events.Publish(ctx, rtevents.ContainerCreated, rtevents.Meta{
		ContainerID: info.ID, DeploymentID: req.DeploymentID, CorrelationID: req.CorrelationID,
	}, nil)
	if l.metrics != nil {
		l.metrics.ObserveCreate(time.Since(start).Seconds())
	}
	return info, nil
}

// Start starts a container.
func (l *Lifecycle) Start(ctx context.Context, containerID, correlationID string) error {
	start := time.Now()
	rec, _ := l.store.GetByContainerID(ctx, containerID)
	if err := l.docker.StartContainer(ctx, containerID); err != nil {
		return err
	}
	_ = l.store.SetStatus(ctx, containerID, rec.Status, model.StatusRunning, "container started")
	_ = l.events.Publish(ctx, rtevents.ContainerStarted, rtevents.Meta{
		ContainerID: containerID, DeploymentID: rec.DeploymentID, CorrelationID: correlationID,
	}, nil)
	if l.metrics != nil {
		l.metrics.ObserveStart(time.Since(start).Seconds())
	}
	return nil
}

// Stop stops a container gracefully.
func (l *Lifecycle) Stop(ctx context.Context, containerID string, timeout time.Duration, correlationID string) error {
	rec, _ := l.store.GetByContainerID(ctx, containerID)
	_ = l.store.SetStatus(ctx, containerID, rec.Status, model.StatusStopping, "stopping")
	if err := l.docker.StopContainer(ctx, containerID, timeout); err != nil {
		return err
	}
	_ = l.store.SetStatus(ctx, containerID, model.StatusStopping, model.StatusStopped, "stopped")
	_ = l.events.Publish(ctx, rtevents.ContainerStopped, rtevents.Meta{
		ContainerID: containerID, DeploymentID: rec.DeploymentID, CorrelationID: correlationID,
	}, nil)
	return nil
}

// Remove stops and removes a container.
func (l *Lifecycle) Remove(ctx context.Context, containerID, correlationID string) error {
	rec, _ := l.store.GetByContainerID(ctx, containerID)
	_ = l.docker.StopContainer(ctx, containerID, 10*time.Second)
	if err := l.docker.RemoveContainer(ctx, containerID); err != nil {
		return err
	}
	_ = l.store.SetStatus(ctx, containerID, rec.Status, model.StatusDeleted, "removed")
	_ = l.events.Publish(ctx, rtevents.ContainerDeleted, rtevents.Meta{
		ContainerID: containerID, DeploymentID: rec.DeploymentID, CorrelationID: correlationID,
	}, nil)
	return nil
}

// Restart restarts a container with policy limits.
func (l *Lifecycle) Restart(ctx context.Context, containerID, correlationID string) error {
	rec, err := l.store.GetByContainerID(ctx, containerID)
	if err != nil {
		return err
	}
	if l.cfg.RestartMaxAttempts > 0 && rec.RestartCount >= l.cfg.RestartMaxAttempts {
		return errors.New(errors.CodeFailedPrecond, "runtime: restart limit exceeded")
	}
	_ = l.store.SetStatus(ctx, containerID, rec.Status, model.StatusRestarting, "restarting")
	if err := l.docker.RestartContainer(ctx, containerID, 10*time.Second); err != nil {
		return err
	}
	_ = l.store.SetStatus(ctx, containerID, model.StatusRestarting, model.StatusRunning, "restarted")
	_ = l.events.Publish(ctx, rtevents.ContainerRestarted, rtevents.Meta{
		ContainerID: containerID, DeploymentID: rec.DeploymentID, CorrelationID: correlationID,
	}, map[string]int{"restart_count": rec.RestartCount + 1})
	return nil
}

// Inspect returns container info from Docker.
func (l *Lifecycle) Inspect(ctx context.Context, containerID string) (model.ContainerInfo, error) {
	return l.docker.InspectContainer(ctx, containerID)
}

// GC prunes unused images.
func (l *Lifecycle) GC(ctx context.Context) (int, error) {
	n, err := l.docker.PruneImages(ctx)
	if err != nil {
		return 0, err
	}
	if n > 0 {
		_ = l.events.Publish(ctx, rtevents.ImageDeleted, rtevents.Meta{}, map[string]int{"count": n})
	}
	return n, nil
}
