// Package events publishes structured platform operations events.
package events

import (
	"context"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/events"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/idx"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
)

// Platform operations event type names.
const (
	UsageUpdated          = "ops.usage.updated"
	QuotaExceeded         = "ops.quota.exceeded"
	QuotaWarning          = "ops.quota.warning"
	ProjectSleeping       = "ops.project.sleeping"
	ProjectAwakened       = "ops.project.awakened"
	BackupCompleted       = "ops.backup.completed"
	BackupFailed          = "ops.backup.failed"
	NotificationDelivered = "ops.notification.delivered"
	NotificationFailed    = "ops.notification.failed"
	SubscriptionRenewed   = "ops.subscription.renewed"
	InvoiceGenerated      = "ops.invoice.generated"
	PlanChanged           = "ops.plan.changed"
	AnalyticsUpdated      = "ops.analytics.updated"
	CleanupCompleted      = "ops.cleanup.completed"
	GarbageCollected      = "ops.gc.completed"
	AuditEventRecorded    = "ops.audit.recorded"
)

// Meta carries contextual identifiers for every ops event.
type Meta struct {
	OrgID         string `json:"org_id,omitempty"`
	ProjectID     string `json:"project_id,omitempty"`
	DeploymentID  string `json:"deployment_id,omitempty"`
	ActorID       string `json:"actor_id,omitempty"`
	CorrelationID string `json:"correlation_id,omitempty"`
}

// Payload wraps the event data with metadata.
type Payload struct {
	Meta Meta `json:"meta"`
	Data any  `json:"data,omitempty"`
}

// Publisher emits platform operations events on the in-process bus.
type Publisher struct {
	bus    events.Bus
	source string
}

// NewPublisher constructs an ops Publisher.
func NewPublisher(bus events.Bus, source string) *Publisher {
	if source == "" {
		source = "worker"
	}
	return &Publisher{bus: bus, source: source}
}

// PublishAsync emits an event without blocking the caller.
func (p *Publisher) PublishAsync(ctx context.Context, name string, meta Meta, data any) error {
	corrID := meta.CorrelationID
	if corrID == "" {
		corrID = logger.CorrelationID(ctx)
	}
	if corrID == "" {
		corrID = idx.NewUUID()
	}
	meta.CorrelationID = corrID

	e, err := events.New(ctx, name, Payload{Meta: meta, Data: data},
		events.WithSource(p.source),
		events.WithCorrelationID(corrID),
		events.WithMetadata(map[string]string{
			"org_id":     meta.OrgID,
			"project_id": meta.ProjectID,
		}),
	)
	if err != nil {
		return err
	}
	return p.bus.PublishAsync(ctx, e)
}

// Publish emits synchronously (critical-path operations).
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
