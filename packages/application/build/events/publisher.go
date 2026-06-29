package events

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
)

// Event type names published by the builder.
const (
	BuildQueued           = "build.queued"
	BuildStarted          = "build.started"
	RepositoryCloned      = "build.repository_cloned"
	FrameworkDetected     = "build.framework_detected"
	DockerfileGenerated   = "build.dockerfile_generated"
	BuildSucceeded        = "build.succeeded"
	BuildFailed           = "build.failed"
	ImagePushed           = "build.image_pushed"
	BuildCancelled        = "build.cancelled"
)

// Meta is common metadata attached to every build event.
type Meta struct {
	EventID       string `json:"event_id"`
	BuildID       string `json:"build_id"`
	DeploymentID  string `json:"deployment_id"`
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	CorrelationID string `json:"correlation_id"`
}

// Payload wraps event data.
type Payload struct {
	Meta Meta        `json:"meta"`
	Data any         `json:"data,omitempty"`
}

// Publisher emits build events to the bus and persists them.
type Publisher struct {
	bus    events.Bus
	source string
	repo   *Repository
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, source string, repo *Repository) *Publisher {
	if source == "" {
		source = "builder"
	}
	return &Publisher{bus: bus, source: source, repo: repo}
}

// Publish emits a build event synchronously to the bus and persists it.
func (p *Publisher) Publish(ctx context.Context, eventType string, meta Meta, data any) error {
	if meta.EventID == "" {
		meta.EventID = idx.NewUUID()
	}
	if p.repo != nil {
		if err := p.repo.Insert(ctx, model.BuildEvent{
			ID: meta.EventID, BuildID: meta.BuildID, DeploymentID: meta.DeploymentID,
			OrgID: meta.OrgID, ProjectID: meta.ProjectID, EventType: eventType,
			CorrelationID: meta.CorrelationID,
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
			"build_id": meta.BuildID, "deployment_id": meta.DeploymentID,
		}),
	)
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, e)
}

// Repository persists builder events.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an event repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Insert stores a build event.
func (r *Repository) Insert(ctx context.Context, ev model.BuildEvent) error {
	meta := ev.Metadata
	if meta == nil {
		meta, _ = json.Marshal(map[string]any{})
	}
	if ev.ID == "" {
		ev.ID = idx.NewUUID()
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO builder_events (id, build_id, deployment_id, org_id, project_id, event_type, correlation_id, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,now())`,
		ev.ID, ev.BuildID, ev.DeploymentID, ev.OrgID, ev.ProjectID, ev.EventType, ev.CorrelationID, meta)
	return postgres.Translate(err, "build events: insert")
}
