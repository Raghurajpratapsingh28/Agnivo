// Package config provides a single, strongly-typed, validated configuration
// system shared by every AgentCloud executable. Configuration is resolved from
// (in increasing order of precedence): built-in defaults, an environment YAML
// file, a .env file, and process environment variables.
package config

import (
	"time"
)

// Environment enumerates the supported deployment environments.
type Environment string

const (
	EnvDevelopment Environment = "development"
	EnvStaging     Environment = "staging"
	EnvProduction  Environment = "production"
)

// IsProduction reports whether the environment is production.
func (e Environment) IsProduction() bool { return e == EnvProduction }

// IsDevelopment reports whether the environment is development.
func (e Environment) IsDevelopment() bool { return e == EnvDevelopment }

// Config is the root configuration shared by all executables.
type Config struct {
	App           App           `mapstructure:"app" validate:"required"`
	HTTP          HTTP          `mapstructure:"http"`
	Log           Log           `mapstructure:"log"`
	Database      Database      `mapstructure:"database"`
	Redis         Redis         `mapstructure:"redis"`
	Observability Observability `mapstructure:"observability"`
	Identity      Identity      `mapstructure:"identity"`
	ControlPlane  ControlPlane  `mapstructure:"controlplane"`
	Builder       Builder       `mapstructure:"builder"`
	Deployer      Deployer      `mapstructure:"deployer"`
	Scheduler     Scheduler     `mapstructure:"scheduler"`
	RuntimeAgent  RuntimeAgent  `mapstructure:"runtime_agent"`
	ProxyManager  ProxyManager  `mapstructure:"proxy_manager"`
	Ops           OpsConfig     `mapstructure:"ops"`
	Security      Security      `mapstructure:"security"`
}

// Security holds platform-wide security settings shared by every executable.
type Security struct {
	// InternalServiceToken protects service-to-service HTTP APIs (/internal/v1
	// and worker internal ports). Required in production when internal APIs are
	// exposed beyond localhost.
	InternalServiceToken string `mapstructure:"internal_service_token"`
	// MetricsBearerToken optionally protects the admin /metrics endpoint.
	MetricsBearerToken string `mapstructure:"metrics_bearer_token"`
	// MaxRequestBodyBytes caps incoming request bodies on public HTTP servers.
	// Zero uses the platform default (10 MiB).
	MaxRequestBodyBytes int64 `mapstructure:"max_request_body_bytes" validate:"gte=0"`
	// EnableHSTS adds Strict-Transport-Security on HTTPS requests in production.
	EnableHSTS bool `mapstructure:"enable_hsts"`
}

// OpsConfig configures the Platform Operations Layer (worker + cron + billing).
type OpsConfig struct {
	// Billing
	BillingEnabled      bool   `mapstructure:"billing_enabled"`
	StripeSecretKey     string `mapstructure:"stripe_secret_key"`
	StripeWebhookSecret string `mapstructure:"stripe_webhook_secret"`
	// SMTP for email notifications.
	SMTPHost string `mapstructure:"smtp_host"`
	SMTPPort int    `mapstructure:"smtp_port"`
	SMTPUser string `mapstructure:"smtp_user"`
	SMTPPass string `mapstructure:"smtp_pass"`
	SMTPFrom string `mapstructure:"smtp_from"`
	// Backup retention.
	BackupRetentionDays int    `mapstructure:"backup_retention_days"`
	BackupStoragePath   string `mapstructure:"backup_storage_path"`
}

// App holds process-level identity and lifecycle configuration.
type App struct {
	// Name is the executable name (api, builder, deployer, ...). Set by bootstrap.
	Name string `mapstructure:"name" validate:"required"`
	// Environment selects environment-specific behavior.
	Environment Environment `mapstructure:"environment" validate:"required,oneof=development staging production"`
	// ShutdownTimeout bounds graceful shutdown.
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" validate:"required,gt=0"`
}

// HTTP configures the public HTTP server. Workers leave Enabled=false and only
// expose the admin server (health + metrics).
type HTTP struct {
	Enabled           bool          `mapstructure:"enabled"`
	Host              string        `mapstructure:"host"`
	Port              int           `mapstructure:"port" validate:"gte=0,lte=65535"`
	ReadTimeout       time.Duration `mapstructure:"read_timeout" validate:"gt=0"`
	ReadHeaderTimeout time.Duration `mapstructure:"read_header_timeout" validate:"gt=0"`
	WriteTimeout      time.Duration `mapstructure:"write_timeout" validate:"gt=0"`
	IdleTimeout       time.Duration `mapstructure:"idle_timeout" validate:"gt=0"`
	RequestTimeout    time.Duration `mapstructure:"request_timeout" validate:"gt=0"`
	CORS              CORS          `mapstructure:"cors"`
}

// CORS configures cross-origin behavior.
type CORS struct {
	AllowedOrigins   []string `mapstructure:"allowed_origins"`
	AllowedMethods   []string `mapstructure:"allowed_methods"`
	AllowedHeaders   []string `mapstructure:"allowed_headers"`
	AllowCredentials bool     `mapstructure:"allow_credentials"`
	MaxAge           int      `mapstructure:"max_age"`
}

// Log configures structured logging.
type Log struct {
	Level  string `mapstructure:"level" validate:"required,oneof=debug info warn error"`
	Format string `mapstructure:"format" validate:"required,oneof=json console"`
}

// Database configures the PostgreSQL connection pool.
type Database struct {
	Enabled         bool          `mapstructure:"enabled"`
	URL             string        `mapstructure:"url" validate:"required_if=Enabled true"`
	MaxConns        int32         `mapstructure:"max_conns" validate:"gte=0"`
	MinConns        int32         `mapstructure:"min_conns" validate:"gte=0"`
	MaxConnLifetime time.Duration `mapstructure:"max_conn_lifetime"`
	MaxConnIdleTime time.Duration `mapstructure:"max_conn_idle_time"`
	ConnectTimeout  time.Duration `mapstructure:"connect_timeout"`
	// QueryTimeout bounds individual queries when callers use the timeout-aware
	// helpers. Zero disables the platform default timeout.
	QueryTimeout time.Duration `mapstructure:"query_timeout"`
	// MaxRetries caps automatic retries of transient (serialization/deadlock/
	// connection) failures. Zero disables automatic retries.
	MaxRetries int `mapstructure:"max_retries" validate:"gte=0"`
	// HealthInterval controls the background pool health monitor cadence. Zero
	// disables the monitor (readiness probes still work on demand).
	HealthInterval time.Duration `mapstructure:"health_interval"`
}

// Redis configures the Redis client.
type Redis struct {
	Enabled      bool          `mapstructure:"enabled"`
	URL          string        `mapstructure:"url" validate:"required_if=Enabled true"`
	PoolSize     int           `mapstructure:"pool_size" validate:"gte=0"`
	MinIdleConns int           `mapstructure:"min_idle_conns" validate:"gte=0"`
	DialTimeout  time.Duration `mapstructure:"dial_timeout"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout"`
}

// Observability configures metrics and tracing.
type Observability struct {
	// AdminHost/AdminPort serve /metrics, /health/live and /health/ready.
	AdminHost string  `mapstructure:"admin_host"`
	AdminPort int     `mapstructure:"admin_port" validate:"gte=0,lte=65535"`
	Tracing   Tracing `mapstructure:"tracing"`
}

// Tracing configures OpenTelemetry trace export.
type Tracing struct {
	Enabled  bool    `mapstructure:"enabled"`
	Exporter string  `mapstructure:"exporter" validate:"omitempty,oneof=otlp stdout"`
	Endpoint string  `mapstructure:"endpoint"`
	Insecure bool    `mapstructure:"insecure"`
	Sampler  float64 `mapstructure:"sampler" validate:"gte=0,lte=1"`
}

// Addr returns the host:port the public HTTP server binds to.
func (h HTTP) Addr() string { return joinHostPort(h.Host, h.Port) }

// AdminAddr returns the host:port the admin server binds to.
func (o Observability) AdminAddr() string { return joinHostPort(o.AdminHost, o.AdminPort) }

// Identity configures the Identity & Access Platform (JWT, tokens, rate limits).
type Identity struct {
	JWT JWT `mapstructure:"jwt"`
}

// JWT configures RS256 access token issuance.
type JWT struct {
	PrivateKeyPEM string        `mapstructure:"private_key_pem"`
	PublicKeyPEM  string        `mapstructure:"public_key_pem"`
	Issuer        string        `mapstructure:"issuer"`
	Audience      string        `mapstructure:"audience"`
	AccessTTL     time.Duration `mapstructure:"access_ttl"`
	RefreshTTL    time.Duration `mapstructure:"refresh_ttl"`
	ClockSkew     time.Duration `mapstructure:"clock_skew"`
}

// ControlPlane configures the project & deployment control plane.
type ControlPlane struct {
	EncryptionKey string        `mapstructure:"encryption_key"`
	DefaultRegion string        `mapstructure:"default_region"`
	Webhooks      WebhookSecrets `mapstructure:"webhooks"`
}

// WebhookSecrets holds HMAC secrets for git provider webhook verification.
type WebhookSecrets struct {
	GitHub    string `mapstructure:"github_secret"`
	GitLab    string `mapstructure:"gitlab_secret"`
	Bitbucket string `mapstructure:"bitbucket_secret"`
}

// Builder configures the build worker executable.
type Builder struct {
	// Concurrency is the number of parallel build jobs per worker instance.
	Concurrency int `mapstructure:"concurrency" validate:"gte=1,lte=256"`
	// WorkspaceDir is the root directory for ephemeral build workspaces.
	WorkspaceDir string `mapstructure:"workspace_dir"`
	// Visibility is the job lease duration for long builds.
	Visibility time.Duration `mapstructure:"visibility" validate:"gt=0"`
	// PollInterval is the idle wait between empty job polls.
	PollInterval time.Duration `mapstructure:"poll_interval" validate:"gt=0"`
	// BuildKitAddr is the BuildKit daemon address (unix:///run/buildkit/buildkitd.sock).
	BuildKitAddr string `mapstructure:"buildkit_addr"`
	// DockerCLI is the docker binary for buildx fallback.
	DockerCLI string `mapstructure:"docker_cli"`
	// ECR configures Amazon Elastic Container Registry integration.
	ECR ECRConfig `mapstructure:"ecr"`
	// Cache configures build cache behavior.
	Cache BuildCacheConfig `mapstructure:"cache"`
	// InternalPort serves internal-only builder HTTP APIs.
	InternalPort int `mapstructure:"internal_port" validate:"gte=0,lte=65535"`
	// TagLatest controls whether images receive a :latest tag in addition to commit/deployment tags.
	TagLatest bool `mapstructure:"tag_latest"`
	// BuilderVersion is stamped into artifact metadata.
	Version string `mapstructure:"version"`
}

// ECRConfig configures registry push targets.
type ECRConfig struct {
	Enabled  bool   `mapstructure:"enabled"`
	Region   string `mapstructure:"region"`
	Registry string `mapstructure:"registry"`
	// RepositoryPrefix is prepended to org/project slugs for ECR repository names.
	RepositoryPrefix string `mapstructure:"repository_prefix"`
}

// BuildCacheConfig tunes layer and registry caching.
type BuildCacheConfig struct {
	Enabled      bool   `mapstructure:"enabled"`
	RegistryRef  string `mapstructure:"registry_ref"`
	InlineCache  bool   `mapstructure:"inline_cache"`
	MaxAgeHours  int    `mapstructure:"max_age_hours" validate:"gte=0"`
}

// Deployer configures the deployment worker executable.
type Deployer struct {
	Concurrency      int           `mapstructure:"concurrency" validate:"gte=1,lte=256"`
	Visibility       time.Duration `mapstructure:"visibility" validate:"gt=0"`
	PollInterval     time.Duration `mapstructure:"poll_interval" validate:"gt=0"`
	DockerCLI        string        `mapstructure:"docker_cli"`
	InternalPort     int           `mapstructure:"internal_port" validate:"gte=0,lte=65535"`
	Version          string        `mapstructure:"version"`
	ECR              ECRConfig     `mapstructure:"ecr"`
	SchedulerURL     string        `mapstructure:"scheduler_url"`
	RuntimeAgentURL  string        `mapstructure:"runtime_agent_url"`
	DefaultStrategy  string        `mapstructure:"default_strategy" validate:"omitempty,oneof=rolling blue_green canary preview immediate"`
	Health           HealthConfig  `mapstructure:"health"`
	Network          NetworkConfig `mapstructure:"network"`
}

// HealthConfig tunes deployment health verification.
type HealthConfig struct {
	StartupTimeout   time.Duration `mapstructure:"startup_timeout" validate:"gt=0"`
	ReadinessTimeout time.Duration `mapstructure:"readiness_timeout" validate:"gt=0"`
	LivenessInterval time.Duration `mapstructure:"liveness_interval" validate:"gt=0"`
	HTTPPath         string        `mapstructure:"http_path"`
	TCPPort          int           `mapstructure:"tcp_port" validate:"gte=0,lte=65535"`
	SuccessThreshold int           `mapstructure:"success_threshold" validate:"gte=1"`
	FailureThreshold int           `mapstructure:"failure_threshold" validate:"gte=1"`
	MaxRetries       int           `mapstructure:"max_retries" validate:"gte=0"`
}

// NetworkConfig configures container networking defaults.
type NetworkConfig struct {
	DockerNetwork string `mapstructure:"docker_network"`
	HostPortMin   int    `mapstructure:"host_port_min" validate:"gte=0"`
	HostPortMax   int    `mapstructure:"host_port_max" validate:"gte=0"`
}

// Scheduler configures the placement scheduler executable.
type Scheduler struct {
	InternalPort        int           `mapstructure:"internal_port" validate:"gte=0,lte=65535"`
	DefaultAlgorithm    string        `mapstructure:"default_algorithm" validate:"omitempty,oneof=first_fit best_fit worst_fit least_loaded balanced region_aware"`
	ReservationTTL      time.Duration `mapstructure:"reservation_ttl" validate:"gt=0"`
	HeartbeatInterval   time.Duration `mapstructure:"heartbeat_interval" validate:"gt=0"`
	MissedHeartbeats    int           `mapstructure:"missed_heartbeats" validate:"gte=1"`
	OvercommitRatio     float64       `mapstructure:"overcommit_ratio" validate:"gte=1"`
	ReconcileInterval   time.Duration `mapstructure:"reconcile_interval" validate:"gt=0"`
	DefaultCPUMillicores int          `mapstructure:"default_cpu_millicores" validate:"gte=1"`
	DefaultMemoryMB     int           `mapstructure:"default_memory_mb" validate:"gte=1"`
	Version             string        `mapstructure:"version"`
}

// ProxyManager configures the edge networking / proxy-manager executable.
type ProxyManager struct {
	// InternalPort serves the proxy-manager internal HTTP API.
	InternalPort int `mapstructure:"internal_port" validate:"gte=0,lte=65535"`
	// CaddyAdminURL is the Caddy Admin API base URL (default http://localhost:2019).
	CaddyAdminURL string `mapstructure:"caddy_admin_url"`
	// PlatformDomain is the base domain for auto-generated platform hostnames.
	PlatformDomain string `mapstructure:"platform_domain"`
	// PreviewDomain is the base domain for auto-generated preview hostnames.
	PreviewDomain string `mapstructure:"preview_domain"`
	// ReconcileInterval is how often the reconciliation loop runs.
	ReconcileInterval time.Duration `mapstructure:"reconcile_interval" validate:"gt=0"`
	// DNSCheckInterval is the minimum delay between DNS verification retries.
	DNSCheckInterval time.Duration `mapstructure:"dns_check_interval" validate:"gt=0"`
	// CertRenewBefore is how far ahead of expiry to trigger certificate renewal.
	CertRenewBefore time.Duration `mapstructure:"cert_renew_before" validate:"gt=0"`
	// PreviewTTL is the default lifetime for preview environments.
	PreviewTTL time.Duration `mapstructure:"preview_ttl" validate:"gt=0"`
	// StreamingChannels configures Redis pub/sub channel prefixes.
	StreamingChannel string `mapstructure:"streaming_channel"`
	// TrustedCIDRs are proxy CIDRs trusted for real-IP extraction.
	TrustedCIDRs []string `mapstructure:"trusted_cidrs"`
	// Version is stamped into proxy metadata.
	Version string `mapstructure:"version"`
}

// RuntimeAgent configures the node runtime agent executable.
type RuntimeAgent struct {
	InternalPort       int           `mapstructure:"internal_port" validate:"gte=0,lte=65535"`
	SchedulerURL       string        `mapstructure:"scheduler_url"`
	AdvertiseHost      string        `mapstructure:"advertise_host"`
	AdvertisePort      int           `mapstructure:"advertise_port" validate:"gte=0,lte=65535"`
	Region             string        `mapstructure:"region"`
	AvailabilityZone   string        `mapstructure:"availability_zone"`
	InstanceType       string        `mapstructure:"instance_type"`
	HeartbeatInterval  time.Duration `mapstructure:"heartbeat_interval" validate:"gt=0"`
	HealthInterval     time.Duration `mapstructure:"health_interval" validate:"gt=0"`
	GCInterval         time.Duration `mapstructure:"gc_interval" validate:"gt=0"`
	DockerHost         string        `mapstructure:"docker_host"`
	DockerNetwork      string        `mapstructure:"docker_network"`
	MaxContainers      int           `mapstructure:"max_containers" validate:"gte=0"`
	CPUCores           float64       `mapstructure:"cpu_cores" validate:"gte=0"`
	MemoryMB           int64         `mapstructure:"memory_mb" validate:"gte=0"`
	DiskGB             int64         `mapstructure:"disk_gb" validate:"gte=0"`
	GPUCount           int           `mapstructure:"gpu_count" validate:"gte=0"`
	RestartMaxAttempts int           `mapstructure:"restart_max_attempts" validate:"gte=0"`
	LogBufferSize      int           `mapstructure:"log_buffer_size" validate:"gte=0"`
	Version            string        `mapstructure:"version"`
}
