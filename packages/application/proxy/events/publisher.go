// Package events publishes structured proxy edge events to the in-process bus
// and persists an audit copy to the database. Every event carries a full set
// of contextual identifiers so consumers can react without additional queries.
package events

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/proxy/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// Proxy edge event type names.
const (
	RouteCreated       = "proxy.route.created"
	RouteUpdated       = "proxy.route.updated"
	RouteDeleted       = "proxy.route.deleted"
	RouteError         = "proxy.route.error"
	DomainVerified     = "proxy.domain.verified"
	DomainVerifyFailed = "proxy.domain.verify_failed"
	CertificateIssued  = "proxy.cert.issued"
	CertificateRenewed = "proxy.cert.renewed"
	CertificateExpired = "proxy.cert.expired"
	CertificateRevoked = "proxy.cert.revoked"
	TrafficSwitched    = "proxy.traffic.switched"
	TrafficRolledBack  = "proxy.traffic.rolled_back"
	PreviewCreated     = "proxy.preview.created"
	PreviewDeleted     = "proxy.preview.deleted"
	PreviewExpired     = "proxy.preview.expired"
	StreamingStarted   = "proxy.streaming.started"
	StreamingStopped   = "proxy.streaming.stopped"
	ReconcileCompleted = "proxy.reconcile.completed"
)

// Meta carries the common context attached to every proxy event.
type Meta struct {
	OrgID         string `json:"org_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	DomainID      string `json:"domain_id,omitempty"`
	RouteID       string `json:"route_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Payload wraps an event's data with its contextual metadata.
type Payload struct {
	Meta Meta `json:"meta"`
	Data any  `json:"data,omitempty"`
}

// Publisher wraps the event bus with proxy-manager conventions and persists
// an audit record for every event.
type Publisher struct {
	bus    events.Bus
	repo   *store.Repository
	source string
}

// NewPublisher constructs a Publisher.
func NewPublisher(bus events.Bus, repo *store.Repository, source string) *Publisher {
	if source == "" {
		source = "proxy-manager"
	}
	return &Publisher{bus: bus, repo: repo, source: source}
}

// PublishAsync emits an event asynchronously and persists an audit record.
func (p *Publisher) PublishAsync(ctx context.Context, name string, meta Meta, data any) error {
	corrID := meta.CorrelationID
	if corrID == "" {
		corrID = logger.CorrelationID(ctx)
	}
	meta.CorrelationID = corrID

	e, err := events.New(ctx, name, Payload{Meta: meta, Data: data},
		events.WithSource(p.source),
		events.WithCorrelationID(corrID),
		events.WithMetadata(map[string]string{
			"org_id":        meta.OrgID,
			"project_id":    meta.ProjectID,
			"deployment_id": meta.DeploymentID,
		}),
	)
	if err != nil {
		return err
	}

	// Persist audit record in the background; do not block the caller.
	go func() {
		mdRaw, _ := json.Marshal(data)
		_ = p.repo.RecordEvent(ctx, model.ProxyEvent{
			ID:            idx.NewUUID(),
			EventType:     name,
			OrgID:         meta.OrgID,
			ProjectID:     meta.ProjectID,
			DeploymentID:  meta.DeploymentID,
			DomainID:      meta.DomainID,
			RouteID:       meta.RouteID,
			CorrelationID: corrID,
			Metadata:      mdRaw,
		})
	}()

	return p.bus.PublishAsync(ctx, e)
}

// Publish emits an event synchronously (for critical path operations).
func (p *Publisher) Publish(ctx context.Context, name string, meta Meta, data any) error {
	corrID := meta.CorrelationID
	if corrID == "" {
		corrID = logger.CorrelationID(ctx)
	}
	meta.CorrelationID = corrID

	e, err := events.New(ctx, name, Payload{Meta: meta, Data: data},
		events.WithSource(p.source),
		events.WithCorrelationID(corrID),
	)
	if err != nil {
		return err
	}
	return p.bus.Publish(ctx, e)
}
