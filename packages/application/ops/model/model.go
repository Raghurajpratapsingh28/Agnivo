// Package model defines all domain types for the Platform Operations Layer:
// billing, metering, quotas, notifications, backups, analytics, audit, and auto-sleep.
package model

import (
	"encoding/json"
	"time"
)

// ─────────────────────────────── Billing ─────────────────────────────────────

// PlanID is a named pricing tier.
type PlanID string

const (
	PlanFree       PlanID = "free"
	PlanPro        PlanID = "pro"
	PlanTeam       PlanID = "team"
	PlanEnterprise PlanID = "enterprise"
)

// BillingInterval is the subscription renewal period.
type BillingInterval string

const (
	IntervalMonthly BillingInterval = "monthly"
	IntervalYearly  BillingInterval = "yearly"
)

// SubscriptionStatus is the state of an org subscription.
type SubscriptionStatus string

const (
	SubStatusActive      SubscriptionStatus = "active"
	SubStatusTrialing    SubscriptionStatus = "trialing"
	SubStatusPastDue     SubscriptionStatus = "past_due"
	SubStatusCanceled    SubscriptionStatus = "canceled"
	SubStatusIncomplete  SubscriptionStatus = "incomplete"
	SubStatusGracePeriod SubscriptionStatus = "grace_period"
)

// InvoiceStatus is the state of a billing invoice.
type InvoiceStatus string

const (
	InvoicePending  InvoiceStatus = "pending"
	InvoicePaid     InvoiceStatus = "paid"
	InvoiceFailed   InvoiceStatus = "failed"
	InvoiceVoided   InvoiceStatus = "voided"
	InvoiceRefunded InvoiceStatus = "refunded"
)

// Plan is a pricing plan definition.
type Plan struct {
	ID              PlanID          `db:"id" json:"id"`
	Name            string          `db:"name" json:"name"`
	PriceCentsMonth int64           `db:"price_cents_month" json:"price_cents_month"`
	PriceCentsYear  int64           `db:"price_cents_year" json:"price_cents_year"`
	StripePriceID   string          `db:"stripe_price_id" json:"stripe_price_id"`
	Features        json.RawMessage `db:"features" json:"features"`
	Active          bool            `db:"active" json:"active"`
	CreatedAt       time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt       time.Time       `db:"updated_at" json:"updated_at"`
}

// Subscription tracks an org's active billing plan.
type Subscription struct {
	ID                  string             `db:"id" json:"id"`
	OrgID               string             `db:"org_id" json:"org_id"`
	PlanID              PlanID             `db:"plan_id" json:"plan_id"`
	Status              SubscriptionStatus `db:"status" json:"status"`
	Interval            BillingInterval    `db:"interval" json:"interval"`
	StripeSubID         string             `db:"stripe_sub_id" json:"stripe_sub_id"`
	StripeCustomerID    string             `db:"stripe_customer_id" json:"stripe_customer_id"`
	CurrentPeriodStart  time.Time          `db:"current_period_start" json:"current_period_start"`
	CurrentPeriodEnd    time.Time          `db:"current_period_end" json:"current_period_end"`
	TrialEndsAt         *time.Time         `db:"trial_ends_at" json:"trial_ends_at,omitempty"`
	CanceledAt          *time.Time         `db:"canceled_at" json:"canceled_at,omitempty"`
	GracePeriodEndsAt   *time.Time         `db:"grace_period_ends_at" json:"grace_period_ends_at,omitempty"`
	CorrelationID       string             `db:"correlation_id" json:"correlation_id"`
	Metadata            json.RawMessage    `db:"metadata" json:"metadata"`
	CreatedAt           time.Time          `db:"created_at" json:"created_at"`
	UpdatedAt           time.Time          `db:"updated_at" json:"updated_at"`
}

// Invoice is a billing statement for an org.
type Invoice struct {
	ID            string          `db:"id" json:"id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	SubscriptionID string         `db:"subscription_id" json:"subscription_id"`
	Status        InvoiceStatus   `db:"status" json:"status"`
	PeriodStart   time.Time       `db:"period_start" json:"period_start"`
	PeriodEnd     time.Time       `db:"period_end" json:"period_end"`
	AmountCents   int64           `db:"amount_cents" json:"amount_cents"`
	CreditsCents  int64           `db:"credits_cents" json:"credits_cents"`
	TaxCents      int64           `db:"tax_cents" json:"tax_cents"`
	TotalCents    int64           `db:"total_cents" json:"total_cents"`
	Currency      string          `db:"currency" json:"currency"`
	StripeInvID   string          `db:"stripe_inv_id" json:"stripe_inv_id"`
	PaidAt        *time.Time      `db:"paid_at" json:"paid_at,omitempty"`
	DueAt         *time.Time      `db:"due_at" json:"due_at,omitempty"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// Credit is a monetary credit applied to an org's account.
type Credit struct {
	ID            string     `db:"id" json:"id"`
	OrgID         string     `db:"org_id" json:"org_id"`
	AmountCents   int64      `db:"amount_cents" json:"amount_cents"`
	UsedCents     int64      `db:"used_cents" json:"used_cents"`
	Reason        string     `db:"reason" json:"reason"`
	CouponCode    string     `db:"coupon_code" json:"coupon_code"`
	ExpiresAt     *time.Time `db:"expires_at" json:"expires_at,omitempty"`
	CorrelationID string     `db:"correlation_id" json:"correlation_id"`
	CreatedAt     time.Time  `db:"created_at" json:"created_at"`
}

// ─────────────────────────────── Metering ────────────────────────────────────

// UsageDimension is the type of resource being measured.
type UsageDimension string

const (
	DimDeployments       UsageDimension = "deployments"
	DimBuildMinutes      UsageDimension = "build_minutes"
	DimContainerHours    UsageDimension = "container_hours"
	DimBandwidthGB       UsageDimension = "bandwidth_gb"
	DimStorageGB         UsageDimension = "storage_gb"
	DimLogGB             UsageDimension = "log_gb"
	DimCPUCoreHours      UsageDimension = "cpu_core_hours"
	DimMemoryGBHours     UsageDimension = "memory_gb_hours"
	DimRequests          UsageDimension = "requests"
	DimCustomDomains     UsageDimension = "custom_domains"
	DimSSLCerts          UsageDimension = "ssl_certs"
	DimProjects          UsageDimension = "projects"
	DimAPIRequests       UsageDimension = "api_requests"
	DimWSConnections     UsageDimension = "ws_connections"
	DimStreamingSessions UsageDimension = "streaming_sessions"
)

// UsageRecord is a single usage measurement event.
type UsageRecord struct {
	ID            string         `db:"id" json:"id"`
	OrgID         string         `db:"org_id" json:"org_id"`
	ProjectID     string         `db:"project_id" json:"project_id"`
	DeploymentID  string         `db:"deployment_id" json:"deployment_id"`
	Dimension     UsageDimension `db:"dimension" json:"dimension"`
	Quantity      float64        `db:"quantity" json:"quantity"`
	Unit          string         `db:"unit" json:"unit"`
	Period        string         `db:"period" json:"period"` // YYYY-MM-DD
	CorrelationID string         `db:"correlation_id" json:"correlation_id"`
	RecordedAt    time.Time      `db:"recorded_at" json:"recorded_at"`
}

// UsageRollup is an aggregated usage summary for a dimension over a period.
type UsageRollup struct {
	ID        string         `db:"id" json:"id"`
	OrgID     string         `db:"org_id" json:"org_id"`
	ProjectID string         `db:"project_id" json:"project_id"`
	Dimension UsageDimension `db:"dimension" json:"dimension"`
	Period    string         `db:"period" json:"period"` // YYYY-MM-DD
	Total     float64        `db:"total" json:"total"`
	Unit      string         `db:"unit" json:"unit"`
	CreatedAt time.Time      `db:"created_at" json:"created_at"`
	UpdatedAt time.Time      `db:"updated_at" json:"updated_at"`
}

// ─────────────────────────────── Quotas ──────────────────────────────────────

// QuotaConfig defines the resource limits for a pricing plan.
type QuotaConfig struct {
	ID                     string    `db:"id" json:"id"`
	PlanID                 PlanID    `db:"plan_id" json:"plan_id"`
	MaxProjects            int64     `db:"max_projects" json:"max_projects"`
	MaxDeployments         int64     `db:"max_deployments" json:"max_deployments"`
	MaxConcurrentBuilds    int64     `db:"max_concurrent_builds" json:"max_concurrent_builds"`
	MaxConcurrentDeploys   int64     `db:"max_concurrent_deploys" json:"max_concurrent_deploys"`
	MaxContainers          int64     `db:"max_containers" json:"max_containers"`
	MaxCustomDomains       int64     `db:"max_custom_domains" json:"max_custom_domains"`
	MaxStorageGB           float64   `db:"max_storage_gb" json:"max_storage_gb"`
	MaxBandwidthGBMonth    float64   `db:"max_bandwidth_gb_month" json:"max_bandwidth_gb_month"`
	MaxBuildMinutesMonth   float64   `db:"max_build_minutes_month" json:"max_build_minutes_month"`
	MaxContainerHoursMonth float64   `db:"max_container_hours_month" json:"max_container_hours_month"`
	MaxAPIRequestsDay      int64     `db:"max_api_requests_day" json:"max_api_requests_day"`
	WarnThresholdPct       float64   `db:"warn_threshold_pct" json:"warn_threshold_pct"`
	CreatedAt              time.Time `db:"created_at" json:"created_at"`
	UpdatedAt              time.Time `db:"updated_at" json:"updated_at"`
}

// QuotaViolation describes an exceeded or approaching limit.
type QuotaViolation struct {
	OrgID     string
	Dimension UsageDimension
	Current   float64
	Limit     float64
	Pct       float64
	IsHard    bool // hard = block operation; soft = warn only
}

// ─────────────────────────────── Notifications ───────────────────────────────

// NotificationChannel is the delivery target.
type NotificationChannel string

const (
	ChannelEmail   NotificationChannel = "email"
	ChannelSlack   NotificationChannel = "slack"
	ChannelDiscord NotificationChannel = "discord"
	ChannelWebhook NotificationChannel = "webhook"
	ChannelInApp   NotificationChannel = "in_app"
)

// NotificationStatus is the delivery state.
type NotificationStatus string

const (
	NotifPending   NotificationStatus = "pending"
	NotifDelivered NotificationStatus = "delivered"
	NotifFailed    NotificationStatus = "failed"
	NotifSkipped   NotificationStatus = "skipped"
)

// Notification is a queued notification for delivery.
type Notification struct {
	ID            string              `db:"id" json:"id"`
	OrgID         string              `db:"org_id" json:"org_id"`
	UserID        string              `db:"user_id" json:"user_id"`
	ProjectID     string              `db:"project_id" json:"project_id"`
	Channel       NotificationChannel `db:"channel" json:"channel"`
	EventType     string              `db:"event_type" json:"event_type"`
	Subject       string              `db:"subject" json:"subject"`
	Body          string              `db:"body" json:"body"`
	Recipient     string              `db:"recipient" json:"recipient"`
	Status        NotificationStatus  `db:"status" json:"status"`
	Attempts      int                 `db:"attempts" json:"attempts"`
	LastAttemptAt *time.Time          `db:"last_attempt_at" json:"last_attempt_at,omitempty"`
	DeliveredAt   *time.Time          `db:"delivered_at" json:"delivered_at,omitempty"`
	FailureReason string              `db:"failure_reason" json:"failure_reason,omitempty"`
	CorrelationID string              `db:"correlation_id" json:"correlation_id"`
	Metadata      json.RawMessage     `db:"metadata" json:"metadata"`
	CreatedAt     time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time           `db:"updated_at" json:"updated_at"`
}

// NotificationPref stores per-org/user delivery preferences.
type NotificationPref struct {
	ID            string              `db:"id" json:"id"`
	OrgID         string              `db:"org_id" json:"org_id"`
	UserID        string              `db:"user_id" json:"user_id"`
	Channel       NotificationChannel `db:"channel" json:"channel"`
	EventType     string              `db:"event_type" json:"event_type"`
	Enabled       bool                `db:"enabled" json:"enabled"`
	Endpoint      string              `db:"endpoint" json:"endpoint"`
	CreatedAt     time.Time           `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time           `db:"updated_at" json:"updated_at"`
}

// ─────────────────────────────── Backup ──────────────────────────────────────

// BackupStatus is the state of a backup job.
type BackupStatus string

const (
	BackupPending   BackupStatus = "pending"
	BackupRunning   BackupStatus = "running"
	BackupCompleted BackupStatus = "completed"
	BackupFailed    BackupStatus = "failed"
	BackupVerified  BackupStatus = "verified"
)

// BackupKind classifies what is being backed up.
type BackupKind string

const (
	BackupDatabase  BackupKind = "database"
	BackupConfig    BackupKind = "config"
	BackupMetadata  BackupKind = "metadata"
)

// Backup is a backup job record.
type Backup struct {
	ID             string          `db:"id" json:"id"`
	Kind           BackupKind      `db:"kind" json:"kind"`
	Status         BackupStatus    `db:"status" json:"status"`
	SizeBytes      int64           `db:"size_bytes" json:"size_bytes"`
	StoragePath    string          `db:"storage_path" json:"storage_path"`
	Checksum       string          `db:"checksum" json:"checksum"`
	DurationSeconds int64          `db:"duration_seconds" json:"duration_seconds"`
	RetentionDays  int             `db:"retention_days" json:"retention_days"`
	ExpiresAt      *time.Time      `db:"expires_at" json:"expires_at,omitempty"`
	VerifiedAt     *time.Time      `db:"verified_at" json:"verified_at,omitempty"`
	FailureReason  string          `db:"failure_reason" json:"failure_reason,omitempty"`
	CorrelationID  string          `db:"correlation_id" json:"correlation_id"`
	Metadata       json.RawMessage `db:"metadata" json:"metadata"`
	StartedAt      *time.Time      `db:"started_at" json:"started_at,omitempty"`
	CompletedAt    *time.Time      `db:"completed_at" json:"completed_at,omitempty"`
	CreatedAt      time.Time       `db:"created_at" json:"created_at"`
}

// ─────────────────────────────── Analytics ───────────────────────────────────

// AnalyticsDaily is a daily aggregated snapshot of platform metrics.
type AnalyticsDaily struct {
	ID                   string    `db:"id" json:"id"`
	Period               string    `db:"period" json:"period"` // YYYY-MM-DD
	ActiveUsers          int64     `db:"active_users" json:"active_users"`
	NewOrgs              int64     `db:"new_orgs" json:"new_orgs"`
	NewProjects          int64     `db:"new_projects" json:"new_projects"`
	TotalDeployments     int64     `db:"total_deployments" json:"total_deployments"`
	SuccessDeployments   int64     `db:"success_deployments" json:"success_deployments"`
	FailedDeployments    int64     `db:"failed_deployments" json:"failed_deployments"`
	TotalBuilds          int64     `db:"total_builds" json:"total_builds"`
	SuccessBuilds        int64     `db:"success_builds" json:"success_builds"`
	FailedBuilds         int64     `db:"failed_builds" json:"failed_builds"`
	AvgBuildDurationSecs int64     `db:"avg_build_duration_secs" json:"avg_build_duration_secs"`
	ActiveContainers     int64     `db:"active_containers" json:"active_containers"`
	TotalBandwidthGB     float64   `db:"total_bandwidth_gb" json:"total_bandwidth_gb"`
	TotalStorageGB       float64   `db:"total_storage_gb" json:"total_storage_gb"`
	TotalAPIRequests     int64     `db:"total_api_requests" json:"total_api_requests"`
	NewSubscriptions     int64     `db:"new_subscriptions" json:"new_subscriptions"`
	MRR                  int64     `db:"mrr_cents" json:"mrr_cents"`
	CreatedAt            time.Time `db:"created_at" json:"created_at"`
	UpdatedAt            time.Time `db:"updated_at" json:"updated_at"`
}

// ─────────────────────────────── Audit ───────────────────────────────────────

// AuditEvent is an immutable audit trail entry.
type AuditEvent struct {
	ID            string          `db:"id" json:"id"`
	OrgID         string          `db:"org_id" json:"org_id"`
	ProjectID     string          `db:"project_id" json:"project_id"`
	ActorID       string          `db:"actor_id" json:"actor_id"`
	ActorType     string          `db:"actor_type" json:"actor_type"` // user, system, api_key
	Action        string          `db:"action" json:"action"`
	ResourceType  string          `db:"resource_type" json:"resource_type"`
	ResourceID    string          `db:"resource_id" json:"resource_id"`
	IPAddress     string          `db:"ip_address" json:"ip_address"`
	UserAgent     string          `db:"user_agent" json:"user_agent"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	Changes       json.RawMessage `db:"changes" json:"changes"`
	Metadata      json.RawMessage `db:"metadata" json:"metadata"`
	OccurredAt    time.Time       `db:"occurred_at" json:"occurred_at"`
}

// ─────────────────────────────── Auto-Sleep ──────────────────────────────────

// SleepStatus is the auto-sleep lifecycle state.
type SleepStatus string

const (
	SleepStatusAwake   SleepStatus = "awake"
	SleepStatusSleeping SleepStatus = "sleeping"
	SleepStatusWaking  SleepStatus = "waking"
)

// SleepEvent records when a project was slept or woken.
type SleepEvent struct {
	ID            string      `db:"id" json:"id"`
	OrgID         string      `db:"org_id" json:"org_id"`
	ProjectID     string      `db:"project_id" json:"project_id"`
	DeploymentID  string      `db:"deployment_id" json:"deployment_id"`
	Status        SleepStatus `db:"status" json:"status"`
	Reason        string      `db:"reason" json:"reason"`
	CorrelationID string      `db:"correlation_id" json:"correlation_id"`
	OccurredAt    time.Time   `db:"occurred_at" json:"occurred_at"`
}

// ─────────────────────────────── Cron ────────────────────────────────────────

// CronJobStatus is the state of a scheduled cron job definition.
type CronJobStatus string

const (
	CronActive   CronJobStatus = "active"
	CronDisabled CronJobStatus = "disabled"
	CronDeleted  CronJobStatus = "deleted"
)

// CronJob is a persistent scheduled job definition.
type CronJob struct {
	ID            string          `db:"id" json:"id"`
	Name          string          `db:"name" json:"name"`
	Schedule      string          `db:"schedule" json:"schedule"` // cron expression
	Timezone      string          `db:"timezone" json:"timezone"`
	JobQueue      string          `db:"job_queue" json:"job_queue"`
	JobType       string          `db:"job_type" json:"job_type"`
	Payload       json.RawMessage `db:"payload" json:"payload"`
	Status        CronJobStatus   `db:"status" json:"status"`
	LastRunAt     *time.Time      `db:"last_run_at" json:"last_run_at,omitempty"`
	NextRunAt     *time.Time      `db:"next_run_at" json:"next_run_at,omitempty"`
	LastError     string          `db:"last_error" json:"last_error,omitempty"`
	CorrelationID string          `db:"correlation_id" json:"correlation_id"`
	CreatedAt     time.Time       `db:"created_at" json:"created_at"`
	UpdatedAt     time.Time       `db:"updated_at" json:"updated_at"`
}

// ─────────────────────────────── Job payloads ────────────────────────────────

// NotifyPayload is the job payload for notification delivery.
type NotifyPayload struct {
	NotificationID string `json:"notification_id"`
	CorrelationID  string `json:"correlation_id"`
}

// BackupPayload is the job payload for a backup task.
type BackupPayload struct {
	Kind          BackupKind `json:"kind"`
	CorrelationID string     `json:"correlation_id"`
}

// CleanupPayload is the job payload for a cleanup task.
type CleanupPayload struct {
	Target        string `json:"target"` // images, containers, sessions, logs, etc.
	CorrelationID string `json:"correlation_id"`
}

// UsageRollupPayload triggers a usage roll-up for a period.
type UsageRollupPayload struct {
	Period        string `json:"period"` // YYYY-MM-DD
	CorrelationID string `json:"correlation_id"`
}

// AnalyticsPayload triggers a daily analytics aggregation.
type AnalyticsPayload struct {
	Period        string `json:"period"` // YYYY-MM-DD
	CorrelationID string `json:"correlation_id"`
}

// SleepPayload triggers auto-sleep for a deployment.
type SleepPayload struct {
	OrgID         string `json:"org_id"`
	ProjectID     string `json:"project_id"`
	DeploymentID  string `json:"deployment_id"`
	CorrelationID string `json:"correlation_id"`
}

// BillingPayload triggers billing cycle processing.
type BillingPayload struct {
	OrgID         string `json:"org_id"`
	CorrelationID string `json:"correlation_id"`
}
