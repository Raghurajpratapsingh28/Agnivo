package config

import (
	"fmt"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/go-playground/validator/v10"
	"github.com/joho/godotenv"
	"github.com/spf13/viper"
)

// EnvPrefix is the prefix for all process environment variables, e.g. AGNIVO_HTTP_PORT.
const EnvPrefix = "AGNIVO"

// Options controls how configuration is loaded.
type Options struct {
	// AppName is the executable name; injected into App.Name.
	AppName string
	// ConfigDir is the directory containing environment YAML files
	// (development.yaml, staging.yaml, production.yaml). Optional.
	ConfigDir string
	// DotEnvPath is the path to a .env file. Optional; defaults to ".env".
	DotEnvPath string
}

// Load resolves configuration from defaults, the environment YAML file, the
// .env file, and process environment variables, then validates the result.
func Load(opts Options) (*Config, error) {
	if opts.AppName == "" {
		return nil, fmt.Errorf("config: AppName is required")
	}

	// .env is loaded first so its values populate the process environment that
	// Viper's AutomaticEnv then reads. Real environment variables already set
	// take precedence (godotenv does not overwrite).
	dotenv := opts.DotEnvPath
	if dotenv == "" {
		dotenv = ".env"
	}
	if _, err := os.Stat(dotenv); err == nil {
		if err := godotenv.Load(dotenv); err != nil {
			return nil, fmt.Errorf("config: load %s: %w", dotenv, err)
		}
	}

	v := viper.New()
	v.SetConfigType("yaml")
	setDefaults(v)

	// Environment-specific YAML overlay.
	env := resolveEnvironment()
	if opts.ConfigDir != "" {
		path := filepath.Join(opts.ConfigDir, string(env)+".yaml")
		if _, err := os.Stat(path); err == nil {
			v.SetConfigFile(path)
			if err := v.ReadInConfig(); err != nil {
				return nil, fmt.Errorf("config: read %s: %w", path, err)
			}
		}
	}

	// Process environment variables have the highest precedence.
	v.SetEnvPrefix(EnvPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()
	bindEnvKeys(v)

	cfg := &Config{}
	// Viper's default decoder applies StringToTimeDurationHookFunc and
	// StringToSliceHookFunc, so durations and comma-lists from env/YAML decode.
	if err := v.Unmarshal(cfg); err != nil {
		return nil, fmt.Errorf("config: unmarshal: %w", err)
	}

	cfg.App.Name = opts.AppName
	cfg.App.Environment = env

	if err := Validate(cfg); err != nil {
		return nil, err
	}
	return cfg, nil
}

// Validate runs struct validation against the configuration.
func Validate(cfg *Config) error {
	validate := validator.New(validator.WithRequiredStructEnabled())
	if err := validate.Struct(cfg); err != nil {
		return fmt.Errorf("config: invalid: %w", err)
	}
	return nil
}

func resolveEnvironment() Environment {
	raw := strings.ToLower(strings.TrimSpace(os.Getenv(EnvPrefix + "_APP_ENVIRONMENT")))
	if raw == "" {
		raw = strings.ToLower(strings.TrimSpace(os.Getenv("APP_ENV")))
	}
	switch Environment(raw) {
	case EnvStaging:
		return EnvStaging
	case EnvProduction:
		return EnvProduction
	default:
		return EnvDevelopment
	}
}

func setDefaults(v *viper.Viper) {
	v.SetDefault("app.shutdown_timeout", 30*time.Second)

	v.SetDefault("http.enabled", false)
	v.SetDefault("http.host", "0.0.0.0")
	v.SetDefault("http.port", 8080)
	v.SetDefault("http.read_timeout", 30*time.Second)
	v.SetDefault("http.read_header_timeout", 10*time.Second)
	v.SetDefault("http.write_timeout", 60*time.Second)
	v.SetDefault("http.idle_timeout", 120*time.Second)
	v.SetDefault("http.request_timeout", 60*time.Second)
	v.SetDefault("http.cors.allowed_origins", []string{"*"})
	v.SetDefault("http.cors.allowed_methods", []string{"GET", "POST", "PUT", "PATCH", "DELETE", "OPTIONS"})
	v.SetDefault("http.cors.allowed_headers", []string{"Authorization", "Content-Type", "X-Request-ID", "X-Correlation-ID"})
	v.SetDefault("http.cors.allow_credentials", false)
	v.SetDefault("http.cors.max_age", 300)

	v.SetDefault("log.level", "info")
	v.SetDefault("log.format", "json")

	v.SetDefault("database.enabled", false)
	v.SetDefault("database.max_conns", 20)
	v.SetDefault("database.min_conns", 2)
	v.SetDefault("database.max_conn_lifetime", time.Hour)
	v.SetDefault("database.max_conn_idle_time", 30*time.Minute)
	v.SetDefault("database.connect_timeout", 10*time.Second)
	v.SetDefault("database.query_timeout", 15*time.Second)
	v.SetDefault("database.max_retries", 3)
	v.SetDefault("database.health_interval", 30*time.Second)

	v.SetDefault("redis.enabled", false)
	v.SetDefault("redis.pool_size", 20)
	v.SetDefault("redis.min_idle_conns", 2)
	v.SetDefault("redis.dial_timeout", 5*time.Second)
	v.SetDefault("redis.read_timeout", 3*time.Second)
	v.SetDefault("redis.write_timeout", 3*time.Second)

	v.SetDefault("observability.admin_host", "0.0.0.0")
	v.SetDefault("observability.admin_port", 9090)
	v.SetDefault("observability.tracing.enabled", false)
	v.SetDefault("observability.tracing.exporter", "otlp")
	v.SetDefault("observability.tracing.sampler", 1.0)

	v.SetDefault("identity.jwt.issuer", "agnivo")
	v.SetDefault("identity.jwt.audience", "agnivo-api")
	v.SetDefault("identity.jwt.access_ttl", 15*time.Minute)
	v.SetDefault("identity.jwt.refresh_ttl", 30*24*time.Hour)
	v.SetDefault("identity.jwt.clock_skew", 30*time.Second)

	v.SetDefault("controlplane.default_region", "us-east-1")

	v.SetDefault("builder.concurrency", 4)
	v.SetDefault("builder.workspace_dir", "/tmp/agnivo-builds")
	v.SetDefault("builder.visibility", 30*time.Minute)
	v.SetDefault("builder.poll_interval", time.Second)
	v.SetDefault("builder.docker_cli", "docker")
	v.SetDefault("builder.internal_port", 8082)
	v.SetDefault("builder.tag_latest", false)
	v.SetDefault("builder.version", "1.0.0")
	v.SetDefault("builder.ecr.enabled", false)
	v.SetDefault("builder.ecr.region", "us-east-1")
	v.SetDefault("builder.ecr.repository_prefix", "agnivo")
	v.SetDefault("builder.cache.enabled", true)
	v.SetDefault("builder.cache.inline_cache", true)
	v.SetDefault("builder.cache.max_age_hours", 168)

	v.SetDefault("deployer.concurrency", 8)
	v.SetDefault("deployer.visibility", 20*time.Minute)
	v.SetDefault("deployer.poll_interval", time.Second)
	v.SetDefault("deployer.docker_cli", "docker")
	v.SetDefault("deployer.internal_port", 8083)
	v.SetDefault("deployer.version", "1.0.0")
	v.SetDefault("deployer.default_strategy", "rolling")
	v.SetDefault("deployer.ecr.enabled", false)
	v.SetDefault("deployer.ecr.region", "us-east-1")
	v.SetDefault("deployer.health.startup_timeout", 5*time.Minute)
	v.SetDefault("deployer.health.readiness_timeout", 3*time.Minute)
	v.SetDefault("deployer.health.liveness_interval", 10*time.Second)
	v.SetDefault("deployer.health.http_path", "/health")
	v.SetDefault("deployer.health.tcp_port", 8080)
	v.SetDefault("deployer.health.success_threshold", 1)
	v.SetDefault("deployer.health.failure_threshold", 3)
	v.SetDefault("deployer.health.max_retries", 5)
	v.SetDefault("deployer.network.docker_network", "agnivo")
	v.SetDefault("deployer.network.host_port_min", 30000)
	v.SetDefault("deployer.network.host_port_max", 40000)

	v.SetDefault("scheduler.internal_port", 8084)
	v.SetDefault("scheduler.default_algorithm", "least_loaded")
	v.SetDefault("scheduler.reservation_ttl", 30*time.Minute)
	v.SetDefault("scheduler.heartbeat_interval", 15*time.Second)
	v.SetDefault("scheduler.missed_heartbeats", 3)
	v.SetDefault("scheduler.overcommit_ratio", 1.0)
	v.SetDefault("scheduler.reconcile_interval", 30*time.Second)
	v.SetDefault("scheduler.default_cpu_millicores", 250)
	v.SetDefault("scheduler.default_memory_mb", 512)
	v.SetDefault("scheduler.version", "1.0.0")

	v.SetDefault("security.max_request_body_bytes", int64(10<<20))
	v.SetDefault("security.enable_hsts", true)

	v.SetDefault("ops.billing_enabled", false)
	v.SetDefault("ops.smtp_port", 587)
	v.SetDefault("ops.smtp_from", "no-reply@agnivo.app")
	v.SetDefault("ops.backup_retention_days", 30)
	v.SetDefault("ops.backup_storage_path", "/var/backups/agnivo")

	v.SetDefault("proxy_manager.internal_port", 8086)
	v.SetDefault("proxy_manager.caddy_admin_url", "http://localhost:2019")
	v.SetDefault("proxy_manager.platform_domain", "agnivo.app")
	v.SetDefault("proxy_manager.preview_domain", "preview.agnivo.app")
	v.SetDefault("proxy_manager.reconcile_interval", 30*time.Second)
	v.SetDefault("proxy_manager.dns_check_interval", 2*time.Minute)
	v.SetDefault("proxy_manager.cert_renew_before", 30*24*time.Hour)
	v.SetDefault("proxy_manager.preview_ttl", 7*24*time.Hour)
	v.SetDefault("proxy_manager.streaming_channel", "proxy")
	v.SetDefault("proxy_manager.trusted_cidrs", []string{"10.0.0.0/8", "172.16.0.0/12", "192.168.0.0/16"})
	v.SetDefault("proxy_manager.version", "1.0.0")

	v.SetDefault("runtime_agent.internal_port", 8085)
	v.SetDefault("runtime_agent.advertise_port", 8085)
	v.SetDefault("runtime_agent.region", "us-east-1")
	v.SetDefault("runtime_agent.heartbeat_interval", 15*time.Second)
	v.SetDefault("runtime_agent.health_interval", 10*time.Second)
	v.SetDefault("runtime_agent.gc_interval", 5*time.Minute)
	v.SetDefault("runtime_agent.docker_network", "agnivo")
	v.SetDefault("runtime_agent.max_containers", 200)
	v.SetDefault("runtime_agent.cpu_cores", 4)
	v.SetDefault("runtime_agent.memory_mb", 8192)
	v.SetDefault("runtime_agent.disk_gb", 100)
	v.SetDefault("runtime_agent.restart_max_attempts", 3)
	v.SetDefault("runtime_agent.log_buffer_size", 4096)
	v.SetDefault("runtime_agent.version", "1.0.0")
}

// bindEnvKeys explicitly binds nested keys so AutomaticEnv resolves them even
// when no config file or default touched the key first.
func bindEnvKeys(v *viper.Viper) {
	keys := []string{
		"app.shutdown_timeout",
		"http.enabled", "http.host", "http.port",
		"http.read_timeout", "http.read_header_timeout", "http.write_timeout",
		"http.idle_timeout", "http.request_timeout",
		"http.cors.allowed_origins", "http.cors.allow_credentials",
		"log.level", "log.format",
		"database.enabled", "database.url", "database.max_conns", "database.min_conns",
		"database.max_conn_lifetime", "database.max_conn_idle_time", "database.connect_timeout",
		"database.query_timeout", "database.max_retries", "database.health_interval",
		"redis.enabled", "redis.url", "redis.pool_size", "redis.min_idle_conns",
		"redis.dial_timeout", "redis.read_timeout", "redis.write_timeout",
		"observability.admin_host", "observability.admin_port",
		"observability.tracing.enabled", "observability.tracing.exporter",
		"observability.tracing.endpoint", "observability.tracing.insecure",
		"observability.tracing.sampler",
		"security.internal_service_token", "security.metrics_bearer_token",
		"security.max_request_body_bytes", "security.enable_hsts",
		"identity.jwt.private_key_pem", "identity.jwt.public_key_pem",
		"identity.jwt.issuer", "identity.jwt.audience",
		"identity.jwt.access_ttl", "identity.jwt.refresh_ttl", "identity.jwt.clock_skew",
		"controlplane.encryption_key", "controlplane.default_region",
		"controlplane.webhooks.github_secret", "controlplane.webhooks.gitlab_secret",
		"controlplane.webhooks.bitbucket_secret",
		"builder.concurrency", "builder.workspace_dir", "builder.visibility", "builder.poll_interval",
		"builder.buildkit_addr", "builder.docker_cli", "builder.internal_port", "builder.tag_latest",
		"builder.version", "builder.ecr.enabled", "builder.ecr.region", "builder.ecr.registry",
		"builder.ecr.repository_prefix", "builder.cache.enabled", "builder.cache.registry_ref",
		"builder.cache.inline_cache", "builder.cache.max_age_hours",
		"deployer.concurrency", "deployer.visibility", "deployer.poll_interval",
		"deployer.docker_cli", "deployer.internal_port", "deployer.version",
		"deployer.default_strategy", "deployer.scheduler_url", "deployer.runtime_agent_url",
		"deployer.ecr.enabled", "deployer.ecr.region", "deployer.ecr.registry",
		"deployer.health.startup_timeout", "deployer.health.readiness_timeout",
		"deployer.health.http_path", "deployer.health.tcp_port",
		"deployer.network.docker_network", "deployer.network.host_port_min", "deployer.network.host_port_max",
		"scheduler.internal_port", "scheduler.default_algorithm", "scheduler.reservation_ttl",
		"scheduler.heartbeat_interval", "scheduler.missed_heartbeats", "scheduler.overcommit_ratio",
		"scheduler.reconcile_interval", "scheduler.default_cpu_millicores", "scheduler.default_memory_mb",
		"ops.billing_enabled", "ops.stripe_secret_key", "ops.stripe_webhook_secret",
		"ops.smtp_host", "ops.smtp_port", "ops.smtp_user", "ops.smtp_pass", "ops.smtp_from",
		"ops.backup_retention_days", "ops.backup_storage_path",
		"proxy_manager.internal_port", "proxy_manager.caddy_admin_url",
		"proxy_manager.platform_domain", "proxy_manager.preview_domain",
		"proxy_manager.reconcile_interval", "proxy_manager.dns_check_interval",
		"proxy_manager.cert_renew_before", "proxy_manager.preview_ttl",
		"proxy_manager.streaming_channel", "proxy_manager.trusted_cidrs",
		"runtime_agent.internal_port", "runtime_agent.scheduler_url", "runtime_agent.advertise_host",
		"runtime_agent.advertise_port", "runtime_agent.region", "runtime_agent.availability_zone",
		"runtime_agent.instance_type", "runtime_agent.heartbeat_interval", "runtime_agent.health_interval",
		"runtime_agent.gc_interval", "runtime_agent.docker_host", "runtime_agent.docker_network",
		"runtime_agent.max_containers", "runtime_agent.cpu_cores", "runtime_agent.memory_mb",
		"runtime_agent.disk_gb", "runtime_agent.gpu_count", "runtime_agent.restart_max_attempts",
	}
	for _, k := range keys {
		_ = v.BindEnv(k)
	}
}

func joinHostPort(host string, port int) string {
	if host == "" {
		host = "0.0.0.0"
	}
	return net.JoinHostPort(host, strconv.Itoa(port))
}
