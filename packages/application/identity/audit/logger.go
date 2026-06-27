package audit

import (
	"context"
	"encoding/json"
	"time"

	"github.com/agnivo/agnivo/packages/platform/database/postgres"
	"github.com/agnivo/agnivo/packages/platform/idx"
	"github.com/agnivo/agnivo/packages/platform/logger"
)

// Entry is an immutable audit log record.
type Entry struct {
	ID            string          `json:"id"`
	OrgID         *string         `json:"org_id,omitempty"`
	UserID        *string         `json:"user_id,omitempty"`
	Action        string          `json:"action"`
	ResourceType  string          `json:"resource_type,omitempty"`
	ResourceID    string          `json:"resource_id,omitempty"`
	Metadata      json.RawMessage `json:"metadata,omitempty"`
	IPAddress     string          `json:"ip_address,omitempty"`
	UserAgent     string          `json:"user_agent,omitempty"`
	RequestID     string          `json:"request_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	CreatedAt     time.Time       `json:"created_at"`
}

// Well-known audit actions.
const (
	ActionLogin           = "auth.login"
	ActionLogout          = "auth.logout"
	ActionRegister        = "auth.register"
	ActionPasswordReset   = "auth.password_reset"
	ActionPasswordChange  = "auth.password_change"
	ActionOrgCreate       = "org.create"
	ActionOrgUpdate       = "org.update"
	ActionOrgDelete       = "org.delete"
	ActionMemberInvite    = "member.invite"
	ActionMemberRemove    = "member.remove"
	ActionMemberRoleChange = "member.role_change"
	ActionAPIKeyCreate    = "apikey.create"
	ActionAPIKeyRotate    = "apikey.rotate"
	ActionAPIKeyDelete    = "apikey.delete"
	ActionSessionRevoke   = "session.revoke"
	ActionTokenRevoke     = "token.revoke"
)

// Logger records audit entries.
type Logger struct{ db *postgres.DB }

// NewLogger constructs an audit logger.
func NewLogger(db *postgres.DB) *Logger { return &Logger{db: db} }

// Record persists an audit entry, extracting request metadata from ctx.
func (l *Logger) Record(ctx context.Context, e Entry) error {
	if e.ID == "" {
		e.ID = idx.NewUUID()
	}
	if e.Metadata == nil {
		e.Metadata, _ = json.Marshal(map[string]any{})
	}
	if e.RequestID == "" {
		e.RequestID = logger.RequestID(ctx)
	}
	if e.CorrelationID == "" {
		e.CorrelationID = logger.CorrelationID(ctx)
	}
	const q = `INSERT INTO identity_audit_logs
		(id, org_id, user_id, action, resource_type, resource_id, metadata, ip_address, user_agent, request_id, correlation_id, created_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,now())`
	_, err := l.db.Conn(ctx).Exec(ctx, q, e.ID, e.OrgID, e.UserID, e.Action, e.ResourceType, e.ResourceID,
		e.Metadata, e.IPAddress, e.UserAgent, e.RequestID, e.CorrelationID)
	return postgres.Translate(err, "audit: record")
}
