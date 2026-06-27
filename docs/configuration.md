# Configuration Guide

All Agnivo executables share a single typed configuration system in `packages/platform/config`.

## Resolution Order

Configuration is merged in increasing precedence:

1. **Built-in defaults** (`packages/platform/config/load.go`)
2. **Environment YAML** — `configs/<environment>.yaml` (via `AGNIVO_CONFIG_DIR`)
3. **`.env` file** — loaded into process environment
4. **Process environment** — `AGNIVO_*` variables (highest precedence)

Environment is selected by `AGNIVO_APP_ENVIRONMENT` or `APP_ENV`: `development`, `staging`, `production`.

## Environment Variable Prefix

All variables use the `AGNIVO_` prefix. Nested keys use underscores:

| Config key | Environment variable |
|------------|---------------------|
| `http.port` | `AGNIVO_HTTP_PORT` |
| `database.url` | `AGNIVO_DATABASE_URL` |
| `identity.jwt.access_ttl` | `AGNIVO_IDENTITY_JWT_ACCESS_TTL` |
| `runtime_agent.internal_port` | `AGNIVO_RUNTIME_AGENT_INTERNAL_PORT` |

## Root Config Struct

```go
type Config struct {
    App           App
    HTTP          HTTP
    Log           Log
    Database      Database
    Redis         Redis
    Observability Observability
    Identity      Identity
    ControlPlane  ControlPlane
    Builder       Builder
    Deployer      Deployer
    Scheduler     Scheduler
    RuntimeAgent  RuntimeAgent
    ProxyManager  ProxyManager
    Ops           OpsConfig
    Security      Security
}
```

---

## App

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `app.environment` | `AGNIVO_APP_ENVIRONMENT` | `development` | `development`, `staging`, `production` |
| `app.shutdown_timeout` | `AGNIVO_APP_SHUTDOWN_TIMEOUT` | `30s` | Graceful shutdown deadline |

`app.name` is set automatically by each executable at boot.

---

## HTTP

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `http.enabled` | `AGNIVO_HTTP_ENABLED` | `false` | Enable public HTTP server |
| `http.host` | `AGNIVO_HTTP_HOST` | `0.0.0.0` | Bind address |
| `http.port` | `AGNIVO_HTTP_PORT` | `8080` | Public port |
| `http.read_timeout` | `AGNIVO_HTTP_READ_TIMEOUT` | `30s` | |
| `http.write_timeout` | `AGNIVO_HTTP_WRITE_TIMEOUT` | `60s` | |
| `http.request_timeout` | `AGNIVO_HTTP_REQUEST_TIMEOUT` | `60s` | Per-request middleware timeout |
| `http.cors.allowed_origins` | `AGNIVO_HTTP_CORS_ALLOWED_ORIGINS` | `["*"]` | Restrict in production |

Workers (`builder`, `deployer`, etc.) leave `http.enabled=false`. Only `api` enables public HTTP.

---

## Database

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `database.enabled` | `AGNIVO_DATABASE_ENABLED` | `false` | Enable PostgreSQL |
| `database.url` | `AGNIVO_DATABASE_URL` | — | Connection string (required if enabled) |
| `database.max_conns` | `AGNIVO_DATABASE_MAX_CONNS` | `20` | Pool max (50 in production overlay) |
| `database.min_conns` | `AGNIVO_DATABASE_MIN_CONNS` | `2` | Pool min |
| `database.query_timeout` | `AGNIVO_DATABASE_QUERY_TIMEOUT` | `15s` | Default query timeout |
| `database.max_retries` | `AGNIVO_DATABASE_MAX_RETRIES` | `3` | Transient failure retries |

Production: use `sslmode=require` in the connection URL.

---

## Redis

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `redis.enabled` | `AGNIVO_REDIS_ENABLED` | `false` | Enable Redis |
| `redis.url` | `AGNIVO_REDIS_URL` | — | Connection URL |
| `redis.pool_size` | `AGNIVO_REDIS_POOL_SIZE` | `20` | Client pool size |

Used for: rate limiting, distributed locks, cron leader election, streaming pub/sub.

---

## Observability

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `observability.admin_host` | `AGNIVO_OBSERVABILITY_ADMIN_HOST` | `0.0.0.0` | Admin server bind |
| `observability.admin_port` | `AGNIVO_OBSERVABILITY_ADMIN_PORT` | `9090` | Health + metrics port |
| `observability.tracing.enabled` | `AGNIVO_OBSERVABILITY_TRACING_ENABLED` | `false` | OpenTelemetry |
| `observability.tracing.exporter` | `AGNIVO_OBSERVABILITY_TRACING_EXPORTER` | `otlp` | `otlp` or `stdout` |
| `observability.tracing.endpoint` | `AGNIVO_OBSERVABILITY_TRACING_ENDPOINT` | — | OTLP collector |
| `observability.tracing.sampler` | `AGNIVO_OBSERVABILITY_TRACING_SAMPLER` | `1.0` | Trace sampling rate (0.1 in prod) |

---

## Security

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `security.internal_service_token` | `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN` | — | **Required in production** |
| `security.metrics_bearer_token` | `AGNIVO_SECURITY_METRICS_BEARER_TOKEN` | — | Protects `/metrics` |
| `security.max_request_body_bytes` | `AGNIVO_SECURITY_MAX_REQUEST_BODY_BYTES` | `10485760` | 10 MiB |
| `security.enable_hsts` | `AGNIVO_SECURITY_ENABLE_HSTS` | `true` | HSTS in production |

---

## Identity (JWT)

| Key | Env Var | Default | Description |
|-----|---------|---------|-------------|
| `identity.jwt.private_key_pem` | `AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM` | auto-dev | RS256 private key |
| `identity.jwt.public_key_pem` | `AGNIVO_IDENTITY_JWT_PUBLIC_KEY_PEM` | auto-dev | RS256 public key |
| `identity.jwt.issuer` | `AGNIVO_IDENTITY_JWT_ISSUER` | `agnivo` | JWT `iss` claim |
| `identity.jwt.audience` | `AGNIVO_IDENTITY_JWT_AUDIENCE` | `agnivo-api` | JWT `aud` claim |
| `identity.jwt.access_ttl` | `AGNIVO_IDENTITY_JWT_ACCESS_TTL` | `15m` | Access token lifetime |
| `identity.jwt.refresh_ttl` | `AGNIVO_IDENTITY_JWT_REFRESH_TTL` | `720h` | Refresh token lifetime |

---

## Control Plane

| Key | Env Var | Description |
|-----|---------|-------------|
| `controlplane.encryption_key` | `AGNIVO_CONTROLPLANE_ENCRYPTION_KEY` | AES key for secrets/env vars |
| `controlplane.default_region` | `AGNIVO_CONTROLPLANE_DEFAULT_REGION` | Default deployment region |
| `controlplane.webhooks.github_secret` | `AGNIVO_CONTROLPLANE_WEBHOOKS_GITHUB_SECRET` | GitHub webhook HMAC |
| `controlplane.webhooks.gitlab_secret` | `AGNIVO_CONTROLPLANE_WEBHOOKS_GITLAB_SECRET` | GitLab webhook HMAC |

---

## Executable-Specific Ports

Each worker exposes an internal HTTP API on a dedicated port (protected by service token):

| Executable | Config key | Default port |
|------------|-----------|--------------|
| builder | `builder.internal_port` | 8082 |
| deployer | `deployer.internal_port` | 8083 |
| scheduler | `scheduler.internal_port` | 8084 |
| runtime-agent | `runtime_agent.internal_port` | 8085 |
| proxy-manager | `proxy_manager.internal_port` | 8086 |

See `.env.example` for the full list of builder, deployer, scheduler, runtime-agent, proxy-manager, and ops settings.

---

## YAML Overlays

| File | Purpose |
|------|---------|
| `configs/development.yaml` | Local dev defaults |
| `configs/production.yaml` | Production timeouts, CORS, tracing sampler |

Secrets must never appear in YAML files — use environment variables only.

## Validation

Configuration is validated at startup via `go-playground/validator`. Invalid config prevents boot with a descriptive error.
