package model

import (
	"encoding/json"
	"time"
)

// BuildStatus is the builder-local lifecycle state.
type BuildStatus string

const (
	StatusQueued    BuildStatus = "queued"
	StatusRunning   BuildStatus = "running"
	StatusSucceeded BuildStatus = "succeeded"
	StatusFailed    BuildStatus = "failed"
	StatusCancelled BuildStatus = "cancelled"
)

// Build is a build execution record.
type Build struct {
	ID            string          `db:"id" json:"id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	JobID         *string         `db:"job_id" json:"job_id,omitempty"`
	Status        BuildStatus     `db:"status" json:"status"`
	Framework     string          `db:"framework" json:"framework"`
	Runtime       string          `db:"runtime" json:"runtime"`
	CommitSHA     string          `db:"commit_sha" json:"commit_sha"`
	Branch        string          `db:"branch" json:"branch"`
	Environment   string          `db:"environment" json:"environment"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	WorkerID      string          `db:"worker_id" json:"worker_id"`
	StartedAt     *time.Time      `db:"started_at" json:"started_at,omitempty"`
	FinishedAt    *time.Time      `db:"finished_at" json:"finished_at,omitempty"`
	FailureReason string          `db:"failure_reason" json:"failure_reason,omitempty"`
	CancelledAt   *time.Time      `db:"cancelled_at" json:"cancelled_at,omitempty"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// LogEntry is a persisted build log line.
type LogEntry struct {
	ID           string          `db:"id" json:"id"`
	BuildID      string          `db:"build_id" json:"build_id"`
	DeploymentID string          `db:"deployment_id" json:"deployment_id"`
	Stage        string          `db:"stage" json:"stage"`
	Level        string          `db:"level" json:"level"`
	Message      string          `db:"message" json:"message"`
	Metadata     json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt    time.Time       `db:"created_at" json:"created_at"`
}

// Artifact is the validated output of a build.
type Artifact struct {
	ID                string          `db:"id" json:"id"`
	BuildID           string          `db:"build_id" json:"build_id"`
	DeploymentID      string          `db:"deployment_id" json:"deployment_id"`
	OrgID             string          `db:"org_id" json:"org_id"`
	ProjectID         string          `db:"project_id" json:"project_id"`
	ImageDigest       string          `db:"image_digest" json:"image_digest"`
	ImageTag          string          `db:"image_tag" json:"image_tag"`
	ImageSizeBytes    int64           `db:"image_size_bytes" json:"image_size_bytes"`
	Registry          string          `db:"registry" json:"registry"`
	Repository        string          `db:"repository" json:"repository"`
	Framework         string          `db:"framework" json:"framework"`
	Runtime           string          `db:"runtime" json:"runtime"`
	DockerfileVersion string          `db:"dockerfile_version" json:"dockerfile_version"`
	BuilderVersion    string          `db:"builder_version" json:"builder_version"`
	CommitSHA         string          `db:"commit_sha" json:"commit_sha"`
	Branch            string          `db:"branch" json:"branch"`
	BuildDurationMs   int64           `db:"build_duration_ms" json:"build_duration_ms"`
	CloneDurationMs   int64           `db:"clone_duration_ms" json:"clone_duration_ms"`
	DockerDurationMs  int64           `db:"docker_duration_ms" json:"docker_duration_ms"`
	PushDurationMs    int64           `db:"push_duration_ms" json:"push_duration_ms"`
	CacheHitRatio     float64         `db:"cache_hit_ratio" json:"cache_hit_ratio"`
	CacheLayersHit    int             `db:"cache_layers_hit" json:"cache_layers_hit"`
	CacheLayersTotal  int             `db:"cache_layers_total" json:"cache_layers_total"`
	Warnings          json.RawMessage `db:"warnings" json:"warnings"`
	SBOM              json.RawMessage `db:"sbom" json:"sbom,omitempty"`
	Metadata          json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt         time.Time       `db:"created_at" json:"created_at"`
}

// BuildEvent is an audit/event record for builds.
type BuildEvent struct {
	ID            string          `db:"id" json:"id"`
	BuildID       string          `db:"build_id" json:"build_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	EventType     string          `db:"event_type" json:"event_type"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}
