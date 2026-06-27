package worker

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	deploycancel "github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/executor"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// Handler processes deployment queue jobs.
type Handler struct {
	pipeline *executor.Pipeline
	cancels  *deploycancel.Registry
}

// NewHandler constructs a deployment job handler.
func NewHandler(pipeline *executor.Pipeline, cancels *deploycancel.Registry) *Handler {
	return &Handler{pipeline: pipeline, cancels: cancels}
}

// Handle dispatches jobs by type.
func (h *Handler) Handle(ctx context.Context, job jobs.Job) error {
	var payload cpjobs.Payload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return errors.Wrap(err, errors.CodeInvalidArgument, "deploy worker: decode payload")
	}
	ctx = executor.WithCorrelation(ctx, payload.CorrelationID)
	log := logger.From(ctx)
	log.Info("processing deploy job", zap.String("job_id", job.ID), zap.String("type", job.Type))

	switch job.Type {
	case cpjobs.TypeDeploy:
		return h.pipeline.RunDeploy(ctx, payload, job.ID)
	case cpjobs.TypeRollback:
		return h.pipeline.RunRollback(ctx, payload, job.ID)
	case cpjobs.TypeDeleteDeployment:
		return h.pipeline.RunCancel(ctx, payload)
	case cpjobs.TypeSleep:
		return h.pipeline.RunSleep(ctx, payload)
	case cpjobs.TypeWake:
		return h.pipeline.RunWake(ctx, payload)
	default:
		return errors.Newf(errors.CodeFailedPrecond, "deploy worker: unknown type %q", job.Type)
	}
}

// CancelDeployment cancels an in-flight deployment.
func (h *Handler) CancelDeployment(deploymentID string) bool {
	return h.cancels.Cancel(deploymentID)
}
