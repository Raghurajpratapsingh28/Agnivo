package model

import (
	"encoding/json"
	"time"
)

// ExecutionStatus is the deployer-local execution state.
type ExecutionStatus string

const (
	ExecQueued    ExecutionStatus = "queued"
	ExecRunning   ExecutionStatus = "running"
	ExecSucceeded ExecutionStatus = "succeeded"
	ExecFailed    ExecutionStatus = "failed"
	ExecCancelled ExecutionStatus = "cancelled"
)

// Phase is a granular deployment pipeline step.
type Phase string

const (
	PhasePending            Phase = "pending"
	PhaseQueued             Phase = "queued"
	PhaseScheduling         Phase = "scheduling"
	PhaseResourcesReserved  Phase = "resources_reserved"
	PhasePreparingRuntime   Phase = "preparing_runtime"
	PhasePullingImage       Phase = "pulling_image"
	PhaseCreatingContainer  Phase = "creating_container"
	PhaseInjectingConfig    Phase = "injecting_configuration"
	PhaseStartingContainer  Phase = "starting_container"
	PhaseWaitingHealth      Phase = "waiting_for_health"
	PhaseSwitchingTraffic   Phase = "switching_traffic"
	PhaseComplete           Phase = "deployment_complete"
	PhaseCleanup            Phase = "cleanup"
	PhaseFailed             Phase = "deployment_failed"
	PhaseRollbackStarted    Phase = "rollback_started"
	PhaseRollbackCompleted  Phase = "rollback_completed"
	PhaseCancelled          Phase = "cancelled"
)

// Strategy names.
const (
	StrategyRolling   = "rolling"
	StrategyBlueGreen = "blue_green"
	StrategyCanary    = "canary"
	StrategyPreview   = "preview"
	StrategyImmediate = "immediate"
)

// Execution is a deployer execution record.
type Execution struct {
	ID               string          `db:"id" json:"id"`
	OrgID            string          `db:"org_id" json:"org_id"`
	ProjectID        string          `db:"project_id" json:"project_id"`
	DeploymentID     string          `db:"deployment_id" json:"deployment_id"`
	JobID            *string         `db:"job_id" json:"job_id,omitempty"`
	Status           ExecutionStatus `db:"status" json:"status"`
	Phase            Phase           `db:"phase" json:"phase"`
	Strategy         string          `db:"strategy" json:"strategy"`
	Environment      string          `db:"environment" json:"environment"`
	ImageTag         string          `db:"image_tag" json:"image_tag"`
	ImageDigest      string          `db:"image_digest" json:"image_digest"`
	BuildID          *string         `db:"build_id" json:"build_id,omitempty"`
	ContainerID      string          `db:"container_id" json:"container_id"`
	Host             string          `db:"host" json:"host"`
	Port             int             `db:"port" json:"port"`
	CorrelationID    string          `db:"correlation_id" json:"correlation_id"`
	WorkerID         string          `db:"worker_id" json:"worker_id"`
	FailureReason    string          `db:"failure_reason" json:"failure_reason,omitempty"`
	RollbackOf       *string         `db:"rollback_of" json:"rollback_of,omitempty"`
	StartedAt        *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt       *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
	DeployDurationMs *int64          `db:"deploy_duration_ms" json:"deploy_duration_ms,omitempty"`
	Metadata         json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
}

// ContainerRecord tracks a managed container instance.
type ContainerRecord struct {
	ID           string     `db:"id" json:"id"`
	ExecutionID  string     `db:"execution_id" json:"execution_id"`
	DeploymentID string     `db:"deployment_id" json:"deployment_id"`
	ContainerID  string     `db:"container_id" json:"container_id"`
	Image        string     `db:"image" json:"image"`
	Host         string     `db:"host" json:"host"`
	Port         int        `db:"port" json:"port"`
	Status       string     `db:"status" json:"status"`
	Role         string     `db:"role" json:"role"`
	CreatedAt    time.Time  `db:"created_at" json:"created_at"`
	StoppedAt    *time.Time `db:"stopped_at" json:"stopped_at,omitempty"`
	RemovedAt    *time.Time `db:"removed_at" json:"removed_at,omitempty"`
}

// HealthRecord is a health check result.
type HealthRecord struct {
	ID           string    `db:"id" json:"id"`
	ExecutionID  string    `db:"execution_id" json:"execution_id"`
	DeploymentID string    `db:"deployment_id" json:"deployment_id"`
	CheckType    string    `db:"check_type" json:"check_type"`
	Success      bool      `db:"success" json:"success"`
	LatencyMs    int64     `db:"latency_ms" json:"latency_ms"`
	Message      string    `db:"message" json:"message"`
	CreatedAt    time.Time `db:"created_at" json:"created_at"`
}

// DeployEvent is a persisted deployer event.
type DeployEvent struct {
	ID            string          `db:"id" json:"id"`
	ExecutionID   string          `db:"execution_id" json:"execution_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	EventType     string          `db:"event_type" json:"event_type"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}

// PhaseTransition records a pipeline phase change.
type PhaseTransition struct {
	ID           string          `db:"id" json:"id"`
	ExecutionID  string          `db:"execution_id" json:"execution_id"`
	DeploymentID string          `db:"deployment_id" json:"deployment_id"`
	FromPhase    string          `db:"from_phase" json:"from_phase"`
	ToPhase      string          `db:"to_phase" json:"to_phase"`
	Message      string          `db:"message" json:"message"`
	Metadata     json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
}

// RuntimeConfig holds container runtime configuration (no secret values in JSON tags for logs).
type RuntimeConfig struct {
	Image       string
	Env         map[string]string
	Secrets     map[string]string
	Labels      map[string]string
	Annotations map[string]string
	Port        int
	Network     string
	HostPort    int
}
