# Agnivo Backend

Modular monolith: **one Go module** (`github.com/agnivo/agnivo`) compiled into
**eight executables**. All business-agnostic infrastructure lives in
`packages/platform` and every binary boots through the same shared bootstrap.

> **Full documentation:** [docs/README.md](../docs/README.md)

## Executables

| Executable | Kind | Default HTTP | Admin (health+metrics) |
|------------|------|--------------|------------------------|
| `api` | HTTP server | 8080 | 9090 |
| `builder` | worker | disabled | 9090 |
| `deployer` | worker | disabled | 9090 |
| `scheduler` | worker | disabled | 9090 |
| `worker` | worker | disabled | 9090 |
| `runtime-agent` | node agent | disabled | 9090 |
| `proxy-manager` | worker | disabled | 9090 |
| `cron` | scheduler | disabled | 9090 |

> Workers keep the public HTTP server disabled; the admin server (liveness,
> readiness, `/metrics`) always runs so orchestrators and Prometheus can reach them.
> In real deployments give each executable a distinct `AGNIVO_OBSERVABILITY_ADMIN_PORT`.

## Shared foundation (`packages/platform`)

| Package | Responsibility |
|---------|----------------|
| `config` | Viper + .env + YAML + `validator`, typed nested config, env overlays |
| `logger` | Zap structured logging, context propagation, request/correlation IDs |
| `observability/metrics` | Prometheus registry (Go/process/HTTP collectors) |
| `observability/tracing` | OpenTelemetry (OTLP/stdout) bootstrap + flush |
| `database/postgres` | pgx pool, transactions (nested savepoints), retries, migrations, metrics |
| `cache/redis` | go-redis client, pub/sub, streams, locks, rate limits, pipelines, metrics |
| `events` | in-process event bus (sync/async, retry, dead-letter, versioning) |
| `jobs` | PostgreSQL SKIP LOCKED job engine (enqueue, lease, heartbeat, DLQ) |
| `repository` | generic CRUD, pagination, optimistic lock, soft delete, bulk ops |
| `errors` | typed errors, codes, HTTP mapping, retry/fatal, logging + trace integration |
| `dto` | request/response envelopes, decode+validate, cursor + offset pagination |
| `validation` | struct validation + domain tags (slug, docker_image, git_repo, secret, …) |
| `httpx` | router, middleware, streaming, downloads, multipart, negotiation, versioning |
| `lifecycle` | ordered start, signal handling, reverse-order graceful shutdown |
| `health` | liveness + readiness registry |
| `bootstrap` | composition root + Google Wire DI + `Run()` |
| `worker` | cancellation-aware poll loop |
| `testkit` | fake config, fake bus/queue, integration helpers, factories, assertions |
| `ptr`, `slicesx`, `strx`, `idx`, `timex`, `ctxx`, `hashx`, `cryptox`, `compress`, `fileutil`, `retry`, `pool`, `cachex` | reusable utilities |

## Run

```bash
# api (HTTP enabled)
AGNIVO_HTTP_ENABLED=true make run-api

# a worker (admin-only)
make run-builder

# build all binaries into ./bin
make build

# tests + vet
make test
make vet
```

## Startup lifecycle (identical for every executable)

`config → logger → metrics → tracing → database → redis → DI (Wire) →
health checks → admin server → register(routes/workers) → public HTTP →
run → graceful shutdown (reverse order, flush traces, close pools, sync logs)`

Each `cmd/<app>/main.go` is a three-line call to `bootstrap.Run(name, app.Register)`.
The only per-executable code is its `internal/app/Register`.

## Configuration

Prefix `AGNIVO_`, `_` for nesting (e.g. `AGNIVO_HTTP_PORT`,
`AGNIVO_DATABASE_URL`). Precedence: defaults < `configs/<env>.yaml` < `.env` <
process env. See [docs/configuration.md](../docs/configuration.md) and `.env.example`.
