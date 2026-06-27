package rollback

import (
	"context"

	"github.com/agnivo/agnivo/packages/application/controlplane/deployment"
	deployevents "github.com/agnivo/agnivo/packages/application/deploy/events"
	"github.com/agnivo/agnivo/packages/application/deploy/runtime"
	"github.com/agnivo/agnivo/packages/platform/errors"
)

// Engine handles automatic and manual rollbacks.
type Engine struct {
	deployments *deployment.Repository
	runtime     runtime.Driver
	events      *deployevents.Publisher
}

// NewEngine constructs a rollback engine.
func NewEngine(deployments *deployment.Repository, rt runtime.Driver, events *deployevents.Publisher) *Engine {
	return &Engine{deployments: deployments, runtime: rt, events: events}
}

// TriggerReason classifies rollback triggers.
type TriggerReason string

const (
	TriggerHealthFailed   TriggerReason = "health_check_failed"
	TriggerContainerCrash TriggerReason = "container_crash"
	TriggerTimeout        TriggerReason = "deployment_timeout"
	TriggerImagePull      TriggerReason = "image_pull_failure"
	TriggerRuntime        TriggerReason = "runtime_failure"
	TriggerConfig         TriggerReason = "configuration_failure"
	TriggerManual         TriggerReason = "manual"
)

// ShouldRollback reports whether failure reason warrants automatic rollback.
func ShouldRollback(reason TriggerReason) bool {
	switch reason {
	case TriggerHealthFailed, TriggerContainerCrash, TriggerRuntime:
		return true
	default:
		return false
	}
}

// RestorePrevious marks rollback intent; actual image restore happens via rollback job pipeline.
func (e *Engine) RestorePrevious(ctx context.Context, orgID, projectID, failedDeploymentID string, meta deployevents.Meta) (deployment.Deployment, error) {
	prev, err := e.deployments.GetPreviousLive(ctx, orgID, projectID, failedDeploymentID)
	if err != nil {
		return deployment.Deployment{}, errors.Wrap(err, errors.CodeNotFound, "rollback: no previous live deployment")
	}
	_ = e.events.Publish(ctx, deployevents.RollbackStarted, meta, map[string]string{
		"target_deployment_id": prev.ID, "image_tag": prev.ImageTag,
	})
	return prev, nil
}

// DrainContainers stops and removes old active containers.
func (e *Engine) DrainContainers(ctx context.Context, containerIDs []string) []string {
	drained := make([]string, 0, len(containerIDs))
	for _, id := range containerIDs {
		_ = e.runtime.Stop(ctx, id, 0)
		if err := e.runtime.Remove(ctx, id); err == nil {
			drained = append(drained, id)
		}
	}
	return drained
}
