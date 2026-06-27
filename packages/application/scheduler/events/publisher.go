package events

import (
	"context"
	"encoding/json"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/events"
	"github.com/agnivo/agnivo/packages/platform/idx"
)

const (
	PlacementRequested   = "scheduler.placement_requested"
	PlacementSucceeded   = "scheduler.placement_succeeded"
	PlacementFailed      = "scheduler.placement_failed"
	ReservationCreated   = "scheduler.reservation_created"
	ReservationReleased  = "scheduler.reservation_released"
	CapacityUpdated      = "scheduler.capacity_updated"
	ServerRegistered     = "scheduler.server_registered"
	ServerOffline        = "scheduler.server_offline"
	ServerRecovered      = "scheduler.server_recovered"
	HeartbeatReceived    = "scheduler.heartbeat_received"
)

// Meta is common scheduler event metadata.
type Meta struct {
	EventID       string `json:"event_id"`
	OrgID         string `json:"org_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	ServerID      string `json:"server_id,omitempty"`
	NodeID        string `json:"node_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Publisher emits scheduler events.
type Publisher struct {
	bus    events.Bus
	source string
	repo   *Repository
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, source string, repo *Repository) *Publisher {
	if source == "" {
		source = "scheduler"
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
	e, err := events.New(ctx, eventType, map[string]any{"meta": meta, "data": data},
		events.WithSource(p.source),
	)
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, e)
}

// Repository persists scheduler events.
type Repository struct{ db *postgres.DB }

// NewRepository constructs an event repository.
func NewRepository(db *postgres.DB) *Repository { return &Repository{db: db} }

// Insert stores a scheduler event.
func (r *Repository) Insert(ctx context.Context, eventType string, meta Meta, data json.RawMessage) error {
	if data == nil {
		data, _ = json.Marshal(map[string]any{})
	}
	_, err := r.db.Conn(ctx).Exec(ctx,
		`INSERT INTO scheduler_events (id, event_type, org_id, project_id, deployment_id, server_id, correlation_id, metadata, created_at)
		VALUES ($1,$2,NULLIF($3,'')::uuid,NULLIF($4,'')::uuid,NULLIF($5,'')::uuid,NULLIF($6,'')::uuid,$7,$8,now())`,
		meta.EventID, eventType, meta.OrgID, meta.ProjectID, meta.DeploymentID, meta.ServerID, meta.CorrelationID, data)
	return postgres.Translate(err, "scheduler events: insert")
}
