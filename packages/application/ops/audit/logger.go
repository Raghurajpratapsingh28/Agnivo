// Package audit implements the enterprise audit trail: every administrative,
// security, and billing action is persisted as an immutable audit event.
package audit

import (
	"context"
	"encoding/json"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/model"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/ops/store"
	"github.com/Raghurajpratapsingh28/Agnivo/packages/platform/logger"
	"go.uber.org/zap"
)

// ActorType identifies the kind of principal performing an action.
const (
	ActorUser   = "user"
	ActorSystem = "system"
	ActorAPIKey = "api_key"
)

// Logger persists audit events and optionally emits structured log lines.
type Logger struct {
	repo   *store.Repository
	log    *zap.Logger
	source string
}

// NewLogger constructs an audit Logger.
func NewLogger(repo *store.Repository, source string, log *zap.Logger) *Logger {
	if source == "" {
		source = "worker"
	}
	return &Logger{repo: repo, log: log, source: source}
}

// RecordInput is the payload for a single audit record.
type RecordInput struct {
	OrgID        string
	ProjectID    string
	ActorID      string
	ActorType    string
	Action       string
	ResourceType string
	ResourceID   string
	IPAddress    string
	UserAgent    string
	Changes      any // will be JSON-marshaled
	Metadata     map[string]any
}

// Record persists an audit event. It never returns an error to callers —
// a failed audit write is logged internally but must not block the operation.
func (l *Logger) Record(ctx context.Context, in RecordInput) {
	changesRaw, _ := json.Marshal(in.Changes)
	metaRaw, _ := json.Marshal(in.Metadata)

	corrID := logger.CorrelationID(ctx)
	actorType := in.ActorType
	if actorType == "" {
		actorType = ActorSystem
	}

	if l.repo == nil {
		l.log.Error("audit: nil repository — event not persisted",
			zap.String("action", in.Action))
		return
	}
	if err := l.repo.RecordAuditEvent(ctx, model.AuditEvent{
		OrgID:         in.OrgID,
		ProjectID:     in.ProjectID,
		ActorID:       in.ActorID,
		ActorType:     actorType,
		Action:        in.Action,
		ResourceType:  in.ResourceType,
		ResourceID:    in.ResourceID,
		IPAddress:     in.IPAddress,
		UserAgent:     in.UserAgent,
		CorrelationID: corrID,
		Changes:       changesRaw,
		Metadata:      metaRaw,
	}); err != nil {
		l.log.Error("audit: failed to persist event",
			zap.String("action", in.Action),
			zap.String("org_id", in.OrgID),
			zap.Error(err))
	}

	// Mirror to structured log for real-time observability.
	l.log.Info("audit",
		zap.String("action", in.Action),
		zap.String("actor_id", in.ActorID),
		zap.String("actor_type", actorType),
		zap.String("org_id", in.OrgID),
		zap.String("resource_type", in.ResourceType),
		zap.String("resource_id", in.ResourceID),
		zap.String("correlation_id", corrID))
}

// List returns recent audit events for an org.
func (l *Logger) List(ctx context.Context, orgID string, limit int) ([]model.AuditEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	return l.repo.ListAuditEvents(ctx, orgID, limit)
}
