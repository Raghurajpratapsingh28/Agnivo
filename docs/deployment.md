# Deployment Guide

## Prerequisites

- Go 1.25+
- PostgreSQL 15+
- Redis 7+
- Docker (for builder, deployer, runtime-agent)
- Caddy (for proxy-manager)

## Configuration

Configuration resolves in order: defaults → `configs/<env>.yaml` → `.env` → `AGNIVO_*` environment variables.

Copy `.env.example` to `.env` and set at minimum:

```bash
AGNIVO_APP_ENVIRONMENT=production
AGNIVO_DATABASE_ENABLED=true
AGNIVO_DATABASE_URL=postgres://...
AGNIVO_REDIS_ENABLED=true
AGNIVO_REDIS_URL=redis://...
AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN=<random-64-bytes>
AGNIVO_SECURITY_METRICS_BEARER_TOKEN=<random-64-bytes>
AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM=...
AGNIVO_IDENTITY_JWT_PUBLIC_KEY_PEM=...
AGNIVO_CONTROLPLANE_ENCRYPTION_KEY=...
```

## Build

```bash
make build          # all 8 executables → bin/
make build-api      # single executable
```

## Docker

```bash
docker compose up -d    # api + web (see docker-compose.yml)
```

The API container exposes:
- **8080** — public HTTP
- **9090** — admin (health + metrics)

Health probe: `GET /health/live` on admin port.

## Production Overlay

Use `configs/production.yaml` with secrets from environment only. Key settings:

- `http.cors.allowed_origins` — restrict to your dashboard domain
- `observability.tracing.sampler` — 0.1 (10%) recommended
- `database.max_conns` — scale with connection pool per instance
- `security.enable_hsts` — true behind TLS terminator

## Multi-Instance Deployment

| Service | Scaling | Notes |
|---------|---------|-------|
| api | Horizontal | Stateless; shared Postgres + Redis |
| builder/deployer/worker | Horizontal | Job queue coordinates via Postgres |
| scheduler | Single leader recommended | Reconcile loop is idempotent |
| runtime-agent | One per host | Registers with scheduler |
| proxy-manager | Horizontal | Redis pub/sub for streaming fan-out |
| cron | Horizontal | Redis leader election |

## Database Migrations

Migrations run automatically at module init via `postgres.DB.Migrate`. Chain order:

`identity → controlplane → build → deploy → scheduler → runtimeagent → proxy → ops`

## Rollback

1. Deploy previous binary version
2. Migrations are forward-only; test migrations in staging before production
3. Use `jobs` dead-letter table for failed async work inspection
