package model

import (
	"encoding/json"
	"time"
)

// HealthStatus is server health.
type HealthStatus string

const (
	HealthHealthy  HealthStatus = "healthy"
	HealthDegraded HealthStatus = "degraded"
	HealthOffline  HealthStatus = "offline"
)

// ReservationStatus tracks reservation lifecycle.
type ReservationStatus string

const (
	ReservationActive   ReservationStatus = "active"
	ReservationReleased ReservationStatus = "released"
	ReservationExpired  ReservationStatus = "expired"
)

// Server is a registered runtime node.
type Server struct {
	ID              string          `db:"id" json:"id"`
	NodeID          string          `db:"node_id" json:"node_id"`
	Hostname        string          `db:"hostname" json:"hostname"`
	AdvertiseHost   string          `db:"advertise_host" json:"advertise_host"`
	AgentURL        string          `db:"agent_url" json:"agent_url"`
	Region          string          `db:"region" json:"region"`
	AvailabilityZone string         `db:"availability_zone" json:"availability_zone"`
	InstanceType    string          `db:"instance_type" json:"instance_type"`
	Architecture    string          `db:"architecture" json:"architecture"`
	OS              string          `db:"os" json:"os"`
	KernelVersion   string          `db:"kernel_version" json:"kernel_version"`
	DockerVersion   string          `db:"docker_version" json:"docker_version"`
	CPUCores        float64         `db:"cpu_cores" json:"cpu_cores"`
	MemoryMB        int64           `db:"memory_mb" json:"memory_mb"`
	DiskGB          int64           `db:"disk_gb" json:"disk_gb"`
	GPUCount        int             `db:"gpu_count" json:"gpu_count"`
	MaxContainers   int             `db:"max_containers" json:"max_containers"`
	ContainerCount  int             `db:"container_count" json:"container_count"`
	ReservedCPU     float64         `db:"reserved_cpu" json:"reserved_cpu"`
	ReservedMemoryMB int64          `db:"reserved_memory_mb" json:"reserved_memory_mb"`
	ReservedDiskGB  int64           `db:"reserved_disk_gb" json:"reserved_disk_gb"`
	HealthStatus    HealthStatus    `db:"health_status" json:"health_status"`
	Labels          json.RawMessage `db:"labels" json:"labels"`
	Metadata        json.RawMessage `db:"metadata" json:"metadata"`
	LastHeartbeat   *time.Time      `db:"last_heartbeat" json:"last_heartbeat,omitempty"`
	MissedBeats     int             `db:"missed_beats" json:"missed_beats"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
}

// AvailableCPU returns unreserved CPU cores.
func (s Server) AvailableCPU(overcommit float64) float64 {
	capacity := s.CPUCores * overcommit
	avail := capacity - s.ReservedCPU
	if avail < 0 {
		return 0
	}
	return avail
}

// AvailableMemoryMB returns unreserved memory.
func (s Server) AvailableMemoryMB(overcommit float64) int64 {
	capacity := int64(float64(s.MemoryMB) * overcommit)
	avail := capacity - s.ReservedMemoryMB
	if avail < 0 {
		return 0
	}
	return avail
}

// AvailableSlots returns remaining container slots.
func (s Server) AvailableSlots() int {
	avail := s.MaxContainers - s.ContainerCount
	if avail < 0 {
		return 0
	}
	return avail
}

// Reservation is a resource hold for a deployment.
type Reservation struct {
	ID            string            `db:"id" json:"id"`
	OrgID         string            `db:"org_id" json:"org_id"`
	ProjectID     string            `db:"project_id" json:"project_id"`
	DeploymentID  string            `db:"deployment_id" json:"deployment_id"`
	ServerID      string            `db:"server_id" json:"server_id"`
	NodeID        string            `db:"node_id" json:"node_id"`
	Host          string            `db:"host" json:"host"`
	Port          int               `db:"port" json:"port"`
	CPUMillicores int               `db:"cpu_millicores" json:"cpu_millicores"`
	MemoryMB      int               `db:"memory_mb" json:"memory_mb"`
	Algorithm     string            `db:"algorithm" json:"algorithm"`
	Status        ReservationStatus `db:"status" json:"status"`
	ExpiresAt     time.Time         `db:"expires_at" json:"expires_at"`
	CorrelationID string            `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage   `db:"metadata" json:"metadata"`
	CreatedAt     time.Time         `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time         `db:"updated_at" json:"updated_at"`
}

// PlacementRequest is input to the scheduling engine.
type PlacementRequest struct {
	OrgID         string
	ProjectID     string
	DeploymentID  string
	Region        string
	AvailabilityZone string
	CPUMillicores int
	MemoryMB      int
	PortMin       int
	PortMax       int
	GPURequired   bool
	Algorithm     string
	Labels        map[string]string
	CorrelationID string
}

// PlacementResult is a successful placement decision.
type PlacementResult struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	NodeID   string `json:"node_id"`
	Region   string `json:"region"`
	AgentURL string `json:"agent_url,omitempty"`
	Reserved bool   `json:"reserved"`
}

// ResourceRequest describes workload resource needs.
type ResourceRequest struct {
	CPUMillicores int
	MemoryMB      int
	DiskMB        int
	GPUCount      int
}

// HeartbeatPayload is sent by runtime agents.
type HeartbeatPayload struct {
	NodeID           string         `json:"node_id"`
	Hostname         string         `json:"hostname"`
	AdvertiseHost    string         `json:"advertise_host"`
	AgentURL         string         `json:"agent_url"`
	Region           string         `json:"region"`
	AvailabilityZone string         `json:"availability_zone"`
	InstanceType     string         `json:"instance_type"`
	Architecture     string         `json:"architecture"`
	OS               string         `json:"os"`
	KernelVersion    string         `json:"kernel_version"`
	DockerVersion    string         `json:"docker_version"`
	CPUCores         float64        `json:"cpu_cores"`
	MemoryMB         int64          `json:"memory_mb"`
	DiskGB           int64          `json:"disk_gb"`
	GPUCount         int           `json:"gpu_count"`
	MaxContainers    int           `json:"max_containers"`
	ContainerCount   int            `json:"container_count"`
	HealthStatus     HealthStatus   `json:"health_status"`
	Labels           map[string]string `json:"labels"`
	Metadata         map[string]any `json:"metadata"`
	LoadSummary      map[string]any `json:"load_summary"`
	ResourceSummary  map[string]any `json:"resource_summary"`
}
