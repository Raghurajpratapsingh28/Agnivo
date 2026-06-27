package events

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/database/postgres"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
)

const (
	ContainerCreated   = "runtime.container_created"
	ContainerStarted   = "runtime.container_started"
	ContainerStopped   = "runtime.container_stopped"
	ContainerRestarted = "runtime.container_restarted"
	ContainerDeleted   = "runtime.container_deleted"
	HealthUpdated      = "runtime.health_updated"
	MetricsCollected   = "runtime.metrics_collected"
	ImagePulled        = "runtime.image_pulled"
	ImageDeleted       = "runtime.image_deleted"
	HeartbeatSent      = "runtime.heartbeat_sent"
)

// Meta is common runtime event metadata.
type Meta struct {
	EventID       string `json:"event_id"`
	ContainerID   string `json:"container_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
	ServerID      string `json:"server_id,omitempty"`
}

// Publisher emits runtime events.
type Publisher struct {
	bus    events.Bus
	source string
	repo   *Repository
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, source string, repo *Repository) *Publisher {
	if source == "" {
		source = "runtime-agent"
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
		if err := p.repo.Insert(ctx, eventType, meta, evMeta); err != nil {
			return err
		}
	}
	if p.bus == nil {
		return nil
	}
	e, err := events.New(ctx, eventType, map[string]any{"meta": meta, "data": data}, events.WithSource(p.source))
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, e)
}

// Repository persists runtime events.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an event repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Insert stores a runtime event.
func (r *Repository) Insert(ctx context.Context, eventType string, meta Meta, data json.RawMessage) error {
	if data == nil {
		data, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO runtime_events (id, event_type, container_id, deployment_id, correlation_id, metadata, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,now())`,
		meta.EventID, eventType, meta.ContainerID, meta.DeploymentID, meta.CorrelationID, data)
	return postgres.Translate(err, "runtime events: insert")
}
