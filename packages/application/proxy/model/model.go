// Package model defines all domain types for the Edge Networking / Proxy Manager layer.
package model

import (
	"encoding/json"
	"time"
)

// RouteStatus is the lifecycle state of a proxy route inside Caddy.
type RouteStatus string

const (
	RouteStatusPending  RouteStatus = "pending"
	RouteStatusActive   RouteStatus = "active"
	RouteStatusDraining RouteStatus = "draining"
	RouteStatusRemoved  RouteStatus = "removed"
	RouteStatusError    RouteStatus = "error"
)

// RouteKind classifies a route's purpose.
type RouteKind string

const (
	RouteKindPlatform RouteKind = "platform"
	RouteKindCustom   RouteKind = "custom"
	RouteKindPreview  RouteKind = "preview"
	RouteKindInternal RouteKind = "internal"
	RouteKindRedirect RouteKind = "redirect"
)

// TrafficMode is the current traffic-routing strategy for a deployment.
type TrafficMode string

const (
	TrafficModeActive   TrafficMode = "active"
	TrafficModeBlue     TrafficMode = "blue"
	TrafficModeGreen    TrafficMode = "green"
	TrafficModeCanary   TrafficMode = "canary"
	TrafficModeDraining TrafficMode = "draining"
)

// CertStatus is the TLS certificate lifecycle state.
type CertStatus string

const (
	CertStatusPending  CertStatus = "pending"
	CertStatusIssuing  CertStatus = "issuing"
	CertStatusActive   CertStatus = "active"
	CertStatusRenewing CertStatus = "renewing"
	CertStatusExpired  CertStatus = "expired"
	CertStatusRevoked  CertStatus = "revoked"
	CertStatusFailed   CertStatus = "failed"
)

// VerifyStatus is the DNS ownership-verification state.
type VerifyStatus string

const (
	VerifyStatusPending  VerifyStatus = "pending"
	VerifyStatusVerified VerifyStatus = "verified"
	VerifyStatusFailed   VerifyStatus = "failed"
	VerifyStatusExpired  VerifyStatus = "expired"
)

// PreviewStatus is the preview environment routing state.
type PreviewStatus string

const (
	PreviewStatusActive  PreviewStatus = "active"
	PreviewStatusExpired PreviewStatus = "expired"
	PreviewStatusDeleted PreviewStatus = "deleted"
)

// Route is an edge routing record that maps a hostname to a backend upstream
// and carries the full Caddy configuration for that vhost.
type Route struct {
	ID               string          `db:"id" json:"id"`
	OrgID            string          `db:"org_id" json:"org_id"`
	ProjectID        string          `db:"project_id" json:"project_id"`
	DeploymentID     string          `db:"deployment_id" json:"deployment_id"`
	DomainID         string          `db:"domain_id" json:"domain_id"`
	Hostname         string          `db:"hostname" json:"hostname"`
	Upstream         string          `db:"upstream" json:"upstream"`
	Kind             RouteKind       `db:"kind" json:"kind"`
	Status           RouteStatus     `db:"status" json:"status"`
	TrafficMode      TrafficMode     `db:"traffic_mode" json:"traffic_mode"`
	CanaryWeight     int             `db:"canary_weight" json:"canary_weight"`
	PreviousUpstream string          `db:"previous_upstream" json:"previous_upstream,omitempty"`
	TLSEnabled       bool            `db:"tls_enabled" json:"tls_enabled"`
	HTTPSRedirect    bool            `db:"https_redirect" json:"https_redirect"`
	StripPrefix      string          `db:"strip_prefix" json:"strip_prefix,omitempty"`
	AddHeaders       json.RawMessage `db:"add_headers" json:"add_headers,omitempty"`
	TimeoutSeconds   int             `db:"timeout_seconds" json:"timeout_seconds"`
	MaxRetries       int             `db:"max_retries" json:"max_retries"`
	Version          int             `db:"version" json:"version"`
	CorrelationID    string          `db:"correlation_id" json:"correlation_id"`
	Metadata         json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt        time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt        time.Time       `db:"updated_at" json:"updated_at"`
	DeletedAt        *time.Time      `db:"deleted_at" json:"deleted_at,omitempty"`
}

// IsActive reports whether the route is actively serving traffic.
func (r Route) IsActive() bool { return r.Status == RouteStatusActive }

// Certificate tracks the full TLS certificate lifecycle for a hostname.
type Certificate struct {
	ID            string          `db:"id" json:"id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	DomainID      string          `db:"domain_id" json:"domain_id"`
	Hostname      string          `db:"hostname" json:"hostname"`
	Status        CertStatus      `db:"status" json:"status"`
	Issuer        string          `db:"issuer" json:"issuer"`
	SerialNumber  string          `db:"serial_number" json:"serial_number"`
	Fingerprint   string          `db:"fingerprint" json:"fingerprint"`
	IssuedAt      *time.Time      `db:"issued_at" json:"issued_at,omitempty"`
	ExpiresAt     *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	RenewAfter    *time.Time      `db:"renew_after" json:"renew_after,omitempty"`
	IsWildcard    bool            `db:"is_wildcard" json:"is_wildcard"`
	AcmeChallenge string          `db:"acme_challenge" json:"acme_challenge,omitempty"`
	FailureReason string          `db:"failure_reason" json:"failure_reason,omitempty"`
	Attempts      int             `db:"attempts" json:"attempts"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// NeedsRenewal reports whether the certificate is due for renewal.
func (c Certificate) NeedsRenewal() bool {
	if c.RenewAfter == nil {
		return false
	}
	return time.Now().UTC().After(*c.RenewAfter)
}

// IsExpired reports whether the certificate has expired.
func (c Certificate) IsExpired() bool {
	if c.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*c.ExpiresAt)
}

// DomainVerification tracks DNS ownership proof for a hostname.
type DomainVerification struct {
	ID             string          `db:"id" json:"id"`
	OrgID          string          `db:"org_id" json:"org_id"`
	ProjectID      string          `db:"project_id" json:"project_id"`
	DomainID       string          `db:"domain_id" json:"domain_id"`
	Hostname       string          `db:"hostname" json:"hostname"`
	Method         string          `db:"method" json:"method"` // txt, cname, a, aaaa
	ChallengeValue string          `db:"challenge_value" json:"challenge_value"`
	Status         VerifyStatus    `db:"status" json:"status"`
	Attempts       int             `db:"attempts" json:"attempts"`
	LastAttemptAt  *time.Time      `db:"last_attempt_at" json:"last_attempt_at,omitempty"`
	VerifiedAt     *time.Time      `db:"verified_at" json:"verified_at,omitempty"`
	ExpiresAt      *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	FailureReason  string          `db:"failure_reason" json:"failure_reason,omitempty"`
	CorrelationID  string          `db:"correlation_id" json:"correlation_id"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt      time.Time       `db:"updated_at" json:"updated_at"`
}

// Preview is a short-lived routing record for a preview / branch deployment.
type Preview struct {
	ID            string          `db:"id" json:"id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	Hostname      string          `db:"hostname" json:"hostname"`
	Upstream      string          `db:"upstream" json:"upstream"`
	Branch        string          `db:"branch" json:"branch"`
	CommitSHA     string          `db:"commit_sha" json:"commit_sha"`
	Status        PreviewStatus   `db:"status" json:"status"`
	ExpiresAt     *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
	DeletedAt     *time.Time      `db:"deleted_at" json:"deleted_at,omitempty"`
}

// IsExpired reports whether the preview has passed its TTL.
func (p Preview) IsExpired() bool {
	if p.ExpiresAt == nil {
		return false
	}
	return time.Now().UTC().After(*p.ExpiresAt)
}

// ProxyEvent is a durable audit record of every significant edge networking action.
type ProxyEvent struct {
	ID            string          `db:"id" json:"id"`
	EventType     string          `db:"event_type" json:"event_type"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	DomainID      string          `db:"domain_id" json:"domain_id"`
	RouteID       string          `db:"route_id" json:"route_id"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}

// RouteVersion records a point-in-time snapshot for rollback.
type RouteVersion struct {
	ID            string          `db:"id" json:"id"`
	RouteID       string          `db:"route_id" json:"route_id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	DeploymentID  string          `db:"deployment_id" json:"deployment_id"`
	Hostname      string          `db:"hostname" json:"hostname"`
	Upstream      string          `db:"upstream" json:"upstream"`
	Version       int             `db:"version" json:"version"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
}

// TrafficSwitchRequest is an atomic traffic-switch operation descriptor.
type TrafficSwitchRequest struct {
	OrgID            string
	ProjectID        string
	DeploymentID     string
	Hostname         string
	NewUpstream      string
	PreviousUpstream string
	Mode             TrafficMode
	CanaryWeight     int
	CorrelationID    string
}

// CaddyRouteConfig is the vhost configuration handed to the Caddy Admin API.
type CaddyRouteConfig struct {
	RouteID        string
	Hostname       string
	Upstream       string
	TLSEnabled     bool
	HTTPSRedirect  bool
	StripPrefix    string
	AddHeaders     map[string]string
	TimeoutSeconds int
	MaxRetries     int
	CanaryWeight   int             // 0 = disabled
	CanaryUpstream string          // alt upstream for canary split
}

// StreamMessage is a message published to a Redis channel for SSE fan-out.
type StreamMessage struct {
	ID            string          `json:"id"`
	Channel       string          `json:"channel"`
	EventType     string          `json:"event_type"`
	OrgID         string          `json:"org_id,omitempty"`
	ProjectID     string          `json:"project_id,omitempty"`
	DeploymentID  string          `json:"deployment_id,omitempty"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload,omitempty"`
	Timestamp     time.Time       `json:"timestamp"`
}

// ConnectionStats is a snapshot of live connection counters.
type ConnectionStats struct {
	SSEConnections       int64 `json:"sse_connections"`
	WebSocketConnections int64 `json:"websocket_connections"`
	ActiveSubscriptions  int64 `json:"active_subscriptions"`
}

// RouteSummary is a compact view of a route for status queries.
type RouteSummary struct {
	Hostname     string      `json:"hostname"`
	Upstream     string      `json:"upstream"`
	Status       RouteStatus `json:"status"`
	TrafficMode  TrafficMode `json:"traffic_mode"`
	CanaryWeight int         `json:"canary_weight,omitempty"`
	TLSEnabled   bool        `json:"tls_enabled"`
	Version      int         `json:"version"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

// CertSummary is a compact certificate view.
type CertSummary struct {
	Hostname  string     `json:"hostname"`
	Status    CertStatus `json:"status"`
	IssuedAt  *time.Time `json:"issued_at,omitempty"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
	Issuer    string     `json:"issuer"`
}

// ProxyMetrics is the current metric snapshot surfaced through the internal API.
type ProxyMetrics struct {
	RouteCount        int64   `json:"route_count"`
	ActiveRoutes      int64   `json:"active_routes"`
	CertCount         int64   `json:"cert_count"`
	ActiveCerts       int64   `json:"active_certs"`
	RenewalsPending   int64   `json:"renewals_pending"`
	VerificationsPending int64 `json:"verifications_pending"`
	PreviewCount      int64   `json:"preview_count"`
	SSEConnections    int64   `json:"sse_connections"`
}
