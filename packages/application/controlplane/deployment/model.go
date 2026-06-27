package deployment

import (
	"encoding/json"
	"time"
)

// Status is the deployment lifecycle state.
type Status string

const (
	StatusPending     Status = "pending"
	StatusQueued      Status = "queued"
	StatusBuilding    Status = "building"
	StatusBuilt       Status = "built"
	StatusScheduling  Status = "scheduling"
	StatusDeploying   Status = "deploying"
	StatusLive        Status = "live"
	StatusFailed      Status = "failed"
	StatusCancelled   Status = "cancelled"
	StatusRollingBack Status = "rolling_back"
	StatusRolledBack  Status = "rolled_back"
)

// Deployment is a deployment record.
type Deployment struct {
	ID               string          `db:"id" json:"id"`
	OrgID            string          `db:"org_id" json:"org_id"`
	ProjectID        string          `db:"project_id" json:"project_id"`
	Status           Status          `db:"status" json:"status"`
	CommitSHA        string          `db:"commit_sha" json:"commit_sha"`
	CommitMessage    string          `db:"commit_message" json:"commit_message"`
	Branch           string          `db:"branch" json:"branch"`
	Author           string          `db:"author" json:"author"`
	ImageTag         string          `db:"image_tag" json:"image_tag"`
	Runtime          string          `db:"runtime" json:"runtime"`
	BuildDurationMs  *int64          `db:"build_duration_ms" json:"build_duration_ms,omitempty"`
	DeployDurationMs *int64          `db:"deploy_duration_ms" json:"deploy_duration_ms,omitempty"`
	FailureReason    string          `db:"failure_reason" json:"failure_reason,omitempty"`
	TriggerSource    string          `db:"trigger_source" json:"trigger_source"`
	TriggerUserID    *string         `db:"trigger_user_id" json:"trigger_user_id,omitempty"`
	Environment      string          `db:"environment" json:"environment"`
	CorrelationID    string          `db:"correlation_id" json:"correlation_id"`
	Metadata         json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
	StartedAt        *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt       *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
}

// Event is a timeline entry for a deployment.
type Event struct {
	ID           string          `db:"id" json:"id"`
	DeploymentID string          `db:"deployment_id" json:"deployment_id"`
	Status       Status          `db:"status" json:"status"`
	Message      string          `db:"message" json:"message"`
	Metadata     json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
}

// IsTerminal reports whether the deployment reached a final state.
func (d Deployment) IsTerminal() bool {
	switch d.Status {
	case StatusLive, StatusFailed, StatusCancelled, StatusRolledBack:
		return true
	default:
		return false
	}
}

// IsActive reports whether the deployment is in progress.
func (d Deployment) IsActive() bool {
	return !d.IsTerminal() && d.Status != StatusPending
}
