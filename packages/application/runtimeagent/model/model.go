package model

import (
	"encoding/json"
	"time"
)

// ContainerStatus is container lifecycle state.
type ContainerStatus string

const (
	StatusCreated   ContainerStatus = "created"
	StatusPreparing ContainerStatus = "preparing"
	StatusStarting  ContainerStatus = "starting"
	StatusRunning   ContainerStatus = "running"
	StatusStopping  ContainerStatus = "stopping"
	StatusStopped   ContainerStatus = "stopped"
	StatusRestarting ContainerStatus = "restarting"
	StatusPaused    ContainerStatus = "paused"
	StatusFailed    ContainerStatus = "failed"
	StatusDeleted   ContainerStatus = "deleted"
)

// ContainerRecord tracks a managed container.
type ContainerRecord struct {
	ID            string          `db:"id" json:"id"`
	ContainerID   string          `db:"container_id" json:"container_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	Name          string          `db:"name" json:"name"`
	Image         string          `db:"image" json:"image"`
	Status        ContainerStatus `db:"status" json:"status"`
	HostPort      int             `db:"host_port" json:"host_port"`
	ContainerPort int             `db:"container_port" json:"container_port"`
	RestartCount  int             `db:"restart_count" json:"restart_count"`
	OOMKilled     bool            `db:"oom_killed" json:"oom_killed"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	StartedAt     *time.Time      `db:"started_at" json:"started_at,omitempty"`
	StoppedAt     *time.Time      `db:"stopped_at" json:"stopped_at,omitempty"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// CreateRequest is input to container creation.
type CreateRequest struct {
	DeploymentID  string            `json:"deployment_id"`
	Image         string            `json:"image"`
	Env           map[string]string `json:"env"`
	Secrets       map[string]string `json:"secrets"`
	Labels        map[string]string `json:"labels"`
	Port          int               `json:"port"`
	HostPort      int               `json:"host_port"`
	Network       string            `json:"network"`
	CorrelationID string            `json:"correlation_id"`
}

// ContainerInfo is runtime container state returned to callers.
type ContainerInfo struct {
	ID       string          `json:"id"`
	Name     string          `json:"name"`
	Host     string          `json:"host"`
	Port     int             `json:"port"`
	Image    string          `json:"image"`
	Status   ContainerStatus `json:"status"`
}

// HealthReport is a container health snapshot.
type HealthReport struct {
	ContainerID string  `json:"container_id"`
	Healthy     bool    `json:"healthy"`
	CPUPercent  float64 `json:"cpu_percent"`
	MemoryMB    int64   `json:"memory_mb"`
	Restarts    int     `json:"restart_count"`
	OOMKilled   bool    `json:"oom_killed"`
	Message     string  `json:"message"`
}

// LogLine is a captured log entry.
type LogLine struct {
	ContainerID   string    `json:"container_id"`
	Stream        string    `json:"stream"`
	Line          string    `json:"line"`
	CorrelationID string    `json:"correlation_id"`
	Timestamp     time.Time `json:"timestamp"`
}
