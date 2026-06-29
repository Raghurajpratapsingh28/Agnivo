package worker

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/cancel"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/executor"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/controlplane/cpjobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/errors"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/jobs"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// Handler processes build.run jobs from the builds queue.
type Handler struct {
	pipeline *executor.Pipeline
	cancels  *cancel.Registry
}

// NewHandler constructs a build job handler.
func NewHandler(pipeline *executor.Pipeline, cancels *cancel.Registry) *Handler {
	return &Handler{pipeline: pipeline, cancels: cancels}
}

// Handle implements jobs.HandlerFunc for build.run.
func (h *Handler) Handle(ctx context.Context, job jobs.Job) error {
	var payload cpjobs.Payload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		return errors.Wrap(err, errors.CodeInvalidArgument, "build worker: decode payload")
	}
	ctx = executor.WithCorrelation(ctx, payload.CorrelationID)
	log := logger.From(ctx)
	log.Info("processing build job",
		zap.String("job_id", job.ID),
		zap.String("deployment_id", payload.DeploymentID),
		zap.String("project_id", payload.ProjectID),
	)
	return h.pipeline.Run(ctx, payload, job.ID)
}

// CancelBuild cancels an in-flight build by deployment ID.
func (h *Handler) CancelBuild(deploymentID string) bool {
	return h.cancels.Cancel(deploymentID)
}
