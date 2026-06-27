# Operations Guide

## Health Checks

Every executable exposes on the **admin port** (default 9090):

| Endpoint | Purpose |
|----------|---------|
| `GET /health/live` | Process is running (liveness) |
| `GET /health/ready` | Dependencies healthy (readiness) |
| `GET /metrics` | Prometheus scrape (optional bearer token) |

Kubernetes example:

```yaml
livenessProbe:
  httpGet:
    path: /health/live
    port: 9090
readinessProbe:
  httpGet:
    path: /health/ready
    port: 9090
```

## Logging

Structured JSON logs in production (`AGNIVO_LOG_FORMAT=json`). Every log line includes:

- `request_id` — per-request UUID
- `correlation_id` — causal chain across services
- `service` — executable name

## Metrics

Prometheus metrics are namespaced `agnivo_*`. Key dashboards:

- `agnivo_http_*` — API latency, status codes, in-flight
- `agnivo_ops_*` — worker throughput, billing, GC
- `agnivo_proxy_*` — routes, certs, DNS verification

Protect `/metrics` with `AGNIVO_SECURITY_METRICS_BEARER_TOKEN` in production.

## Tracing

Enable OpenTelemetry export:

```bash
AGNIVO_OBSERVABILITY_TRACING_ENABLED=true
AGNIVO_OBSERVABILITY_TRACING_EXPORTER=otlp
AGNIVO_OBSERVABILITY_TRACING_ENDPOINT=otel-collector:4318
AGNIVO_OBSERVABILITY_TRACING_SAMPLER=0.1
```

## Background Jobs

Inspect job queue:

```sql
SELECT queue, type, status, count(*) FROM jobs GROUP BY 1,2,3;
SELECT * FROM jobs WHERE status='dead' ORDER BY updated_at DESC LIMIT 20;
```

Workers reclaim expired leases automatically. Dead-letter jobs are purged by the ops GC worker after 7 days.

## Backups

Ops worker runs daily database backups. Configure retention:

```bash
AGNIVO_OPS_BACKUP_RETENTION_DAYS=30
AGNIVO_OPS_BACKUP_STORAGE_PATH=/var/backups/agnivo
```

## Incident Response

1. Check `/health/ready` on affected service
2. Inspect structured logs filtered by `correlation_id`
3. Check Prometheus error rate dashboards
4. Inspect `ops_audit_events` for recent admin actions
5. See [Recovery Guide](recovery.md)

## Cron Schedules

Built-in schedules (registered at cron startup):

| Schedule | Job | Queue |
|----------|-----|-------|
| `@daily` | Usage rollup, analytics, backup | ops, analytics, backup |
| `@hourly` | Notification drain, cleanup | notifications, cleanup |
| `@monthly` | Billing cycle | billing |

Only the Redis-elected cron leader fires jobs.
