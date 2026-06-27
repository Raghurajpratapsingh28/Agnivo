package cpjobs

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/platform/jobs"
)

// Queue names.
const (
	QueueBuilds       = "builds"
	QueueDeployments  = "deployments"
	QueueDomains      = "domains"
)

// Job types consumed by builder, deployer, and proxy-manager.
const (
	TypeBuild             = "build.run"
	TypeDeploy            = "deploy.run"
	TypeRollback          = "deploy.rollback"
	TypeDeleteDeployment  = "deploy.delete"
	TypeSleep             = "project.sleep"
	TypeWake              = "project.wake"
	TypeDomainVerify      = "domain.verify"
	TypeSSLRequest        = "domain.ssl_request"
)

// Payload is the standard job envelope.
type Payload struct {
	OrgID         string          `json:"org_id"`
	ProjectID     string          `json:"project_id"`
	DeploymentID  string          `json:"deployment_id,omitempty"`
	DomainID      string          `json:"domain_id,omitempty"`
	UserID        string          `json:"user_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
}

// Enqueuer wraps the jobs queue with control-plane conventions.
type Enqueuer struct {
	q *jobs.Queue
}

// NewEnqueuer constructs an Enqueuer.
func NewEnqueuer(q *jobs.Queue) *Enqueuer { return &Enqueuer{q: q} }

// EnqueueBuild enqueues a build job.
func (e *Enqueuer) EnqueueBuild(ctx context.Context, p Payload, idempotencyKey string) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueBuilds, TypeBuild, p, jobs.EnqueueOptions{
		Priority: 1, IdempotencyKey: idempotencyKey, MaxAttempts: 25,
	})
}

// EnqueueDeploy enqueues a deploy job.
func (e *Enqueuer) EnqueueDeploy(ctx context.Context, p Payload, idempotencyKey string) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDeployments, TypeDeploy, p, jobs.EnqueueOptions{
		Priority: 2, IdempotencyKey: idempotencyKey, MaxAttempts: 25,
	})
}

// EnqueueRollback enqueues a rollback job.
func (e *Enqueuer) EnqueueRollback(ctx context.Context, p Payload, idempotencyKey string) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDeployments, TypeRollback, p, jobs.EnqueueOptions{
		Priority: 3, IdempotencyKey: idempotencyKey,
	})
}

// EnqueueCancel enqueues a deployment cancellation job.
func (e *Enqueuer) EnqueueCancel(ctx context.Context, p Payload) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDeployments, TypeDeleteDeployment, p, jobs.EnqueueOptions{Priority: 4})
}

// EnqueueSleep enqueues a project sleep job.
func (e *Enqueuer) EnqueueSleep(ctx context.Context, p Payload) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDeployments, TypeSleep, p, jobs.EnqueueOptions{})
}

// EnqueueWake enqueues a project wake job.
func (e *Enqueuer) EnqueueWake(ctx context.Context, p Payload) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDeployments, TypeWake, p, jobs.EnqueueOptions{})
}

// EnqueueDomainVerify enqueues domain DNS verification.
func (e *Enqueuer) EnqueueDomainVerify(ctx context.Context, p Payload) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDomains, TypeDomainVerify, p, jobs.EnqueueOptions{
		Delay: 30 * time.Second,
	})
}

// EnqueueSSLRequest enqueues SSL certificate provisioning.
func (e *Enqueuer) EnqueueSSLRequest(ctx context.Context, p Payload) (jobs.Job, error) {
	return e.q.Enqueue(ctx, QueueDomains, TypeSSLRequest, p, jobs.EnqueueOptions{})
}
