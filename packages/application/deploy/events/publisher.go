package events

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/deploy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
)

const (
	DeploymentQueued    = "deploy.queued"
	ResourcesReserved   = "deploy.resources_reserved"
	ImagePulled         = "deploy.image_pulled"
	ContainerCreated    = "deploy.container_created"
	DeploymentStarted   = "deploy.started"
	DeploymentSucceeded = "deploy.succeeded"
	DeploymentFailed    = "deploy.failed"
	DeploymentCancelled = "deploy.cancelled"
	HealthCheckPassed   = "deploy.health_passed"
	HealthCheckFailed   = "deploy.health_failed"
	RollbackStarted     = "deploy.rollback_started"
	RollbackSucceeded   = "deploy.rollback_succeeded"
	RollbackFailed      = "deploy.rollback_failed"
	ContainerStopped    = "deploy.container_stopped"
	ContainerRemoved    = "deploy.container_removed"
	TrafficSwitched     = "deploy.traffic_switched"
)

// Meta is common event metadata.
type Meta struct {
	EventID       string `json:"event_id"`
	ExecutionID   string `json:"execution_id"`
	DeploymentID  string `json:"deployment_id"`
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	CorrelationID string `json:"correlation_id"`
}

// Payload wraps event data.
type Payload struct {
	Meta Meta `json:"meta"`
	Data any  `json:"data,omitempty"`
}

// Publisher emits deploy events.
type Publisher struct {
	bus    events.Bus
	source string
	repo   *Repository
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, source string, repo *Repository) *Publisher {
	if source == "" {
		source = "deployer"
	}
	return &Publisher{bus: bus, source: source, repo: repo}
}

// Publish emits and persists an event.
func (p *Publisher) Publish(ctx context.Context, eventType string, meta Meta, data any) error {
	if meta.EventID == "" {
		meta.EventID = idx.NewUUID()
	}
	if p.repo != nil {
		evMeta, _ := json.Marshal(data)
		if err := p.repo.Insert(ctx, model.DeployEvent{
			ID: meta.EventID, ExecutionID: meta.ExecutionID, DeploymentID: meta.DeploymentID,
			OrgID: meta.OrgID, ProjectID: meta.ProjectID, EventType: eventType, CorrelationID: meta.CorrelationID,
			Metadata: evMeta,
		}); err != nil {
			return err
		}
	}
	if p.bus == nil {
		return nil
	}
	e, err := events.New(ctx, eventType, Payload{Meta: meta, Data: data},
		events.WithSource(p.source),
		events.WithMetadata(map[string]string{
			"org_id": meta.OrgID, "project_id": meta.ProjectID,
			"deployment_id": meta.DeploymentID, "execution_id": meta.ExecutionID,
		}),
	)
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, e)
}

// Repository persists deployer events.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an event repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Insert stores a deploy event.
func (r *Repository) Insert(ctx context.Context, ev model.DeployEvent) error {
	meta := ev.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	if ev.ID == "" {
		ev.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO deployer_events (id, execution_id, deployment_id, org_id, project_id, event_type, correlation_id, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())`,
		ev.ID, ev.ExecutionID, ev.DeploymentID, ev.OrgID, ev.ProjectID, ev.EventType, ev.CorrelationID, meta)
	return postgres.Translate(err, "deploy events: insert")
}
