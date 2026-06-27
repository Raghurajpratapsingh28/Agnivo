package cpevents

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
)

// Event names for the control plane.
const (
	ProjectCreated         = "project.created"
	ProjectUpdated         = "project.updated"
	ProjectDeleted         = "project.deleted"
	ProjectArchived        = "project.archived"
	RepositoryConnected    = "repository.connected"
	RepositoryDisconnected = "repository.disconnected"
	DeploymentRequested    = "deployment.requested"
	DeploymentCancelled    = "deployment.cancelled"
	DeploymentSucceeded    = "deployment.succeeded"
	DeploymentFailed       = "deployment.failed"
	SecretUpdated          = "secret.updated"
	EnvironmentUpdated     = "environment.updated"
	DomainAdded            = "domain.added"
	DomainRemoved          = "domain.removed"
	WebhookReceived        = "webhook.received"
)

// Publisher wraps the event bus with control-plane conventions.
type Publisher struct {
	bus    events.Bus
	source string
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, source string) *Publisher {
	if source == "" {
		source = "api"
	}
	return &Publisher{bus: bus, source: source}
}

// Meta is common event metadata.
type Meta struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id,omitempty"`
	AggregateID   string `json:"aggregate_id"`
	ActorID       string `json:"actor_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Payload wraps domain data with metadata.
type Payload struct {
	Meta Meta `json:"meta"`
	Data any  `json:"data,omitempty"`
}

// PublishAsync emits an event asynchronously.
func (p *Publisher) PublishAsync(ctx context.Context, name string, meta Meta, data any) error {
	e, err := events.New(ctx, name, Payload{Meta: meta, Data: data},
		events.WithSource(p.source),
		events.WithMetadata(map[string]string{
			"org_id": meta.OrgID, "project_id": meta.ProjectID, "actor_id": meta.ActorID,
		}),
	)
	if err != nil {
		return err
	}
	return p.bus.PublishAsync(ctx, e)
}
