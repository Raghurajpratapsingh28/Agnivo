# Platform Packages Reference

Shared infrastructure in `packages/platform/`. These packages contain **no business logic** and are used by every executable.

## Core

| Package | Import path | Purpose |
|---------|-------------|---------|
| **bootstrap** | `packages/platform/bootstrap` | Composition root: config → DI → lifecycle → HTTP servers |
| **config** | `packages/platform/config` | Typed configuration (Viper + validator) |
| **lifecycle** | `packages/platform/lifecycle` | Signal handling, ordered start/stop, graceful shutdown |
| **worker** | `packages/platform/worker` | Cancellation-aware poll loop helper |

## HTTP

| Package | Purpose |
|---------|---------|
| **httpx** | Router, middleware chain, pagination, streaming, multipart |
| **httpx/middleware** | RequestID, CorrelationID, Recovery, Logger, SecurityHeaders, CORS, BearerToken, MaxBodyBytes |
| **dto** | JSON request/response envelopes, decode+validate |
| **health** | Liveness and readiness probe registry |

## Data

| Package | Purpose |
|---------|---------|
| **database/postgres** | pgx pool, transactions, savepoints, migrations, retries, metrics |
| **cache/redis** | go-redis client, distributed locks, pub/sub, streams, token buckets |
| **repository** | Generic CRUD, pagination, optimistic locking, soft delete |
| **jobs** | PostgreSQL SKIP LOCKED job queue, worker pools, DLQ, idempotency |

## Observability

| Package | Purpose |
|---------|---------|
| **logger** | Zap structured logging with context propagation |
| **observability/metrics** | Prometheus registry, HTTP middleware collectors |
| **observability/tracing** | OpenTelemetry OTLP/stdout export |

## Reliability

| Package | Purpose |
|---------|---------|
| **errors** | Typed error codes, HTTP mapping, retry classification |
| **retry** | Context-aware retry with exponential backoff + jitter |
| **resilience** | Circuit breaker for external dependencies |
| **events** | In-process event bus (sync/async, retry, dead-letter) |
| **pool** | Bounded worker pools |

## Utilities

| Package | Purpose |
|---------|---------|
| **validation** | Struct validation + domain tags (slug, docker_image, git_repo) |
| **idx** | UUID and prefixed ID generation |
| **cryptox** | AES-GCM, key derivation |
| **hashx** | Consistent hashing |
| **compress** | gzip helpers |
| **fileutil** | Safe file operations |
| **ctxx** | Context value helpers |
| **ptr** | Generic pointer helpers |
| **slicesx** | Slice utilities |
| **strx** | String utilities |
| **timex** | Time parsing and formatting |
| **cachex** | Generic in-memory cache |
| **testkit** | Test fixtures, fake bus/queue, integration helpers |

## Bootstrap Lifecycle

Every executable follows the same boot sequence:

```
config.Load → validate
  → logger
  → metrics registry
  → tracing provider
  → postgres pool (+ migrate)
  → redis client
  → health registry
  → admin server (:9090)
  → Register(routes, workers, hooks)
  → public HTTP server (if enabled)
  → lifecycle.Run() until SIGTERM
  → graceful shutdown (reverse order)
```

## Adding a Platform Package

1. Create `packages/platform/<name>/`
2. Keep it business-logic-free
3. Export a minimal public API
4. Add tests alongside the package
5. Wire through bootstrap if needed globally

See [Coding Standards](coding-standards.md) for conventions.
