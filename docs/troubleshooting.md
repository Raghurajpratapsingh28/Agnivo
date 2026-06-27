# Troubleshooting Guide

Common issues and how to diagnose them.

## Startup Failures

### Config validation error

```
config: invalid: Key: 'Config.Database.URL' Error:Field validation for 'URL' failed on the 'required_if' tag
```

**Cause:** Database enabled but `AGNIVO_DATABASE_URL` not set.

**Fix:** Set the URL or disable with `AGNIVO_DATABASE_ENABLED=false`.

### JWT key error in production

**Cause:** `AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM` empty in production.

**Fix:** Generate RS256 key pair and set both private and public PEM env vars.

### Port already in use

```
listen tcp :8080: bind: address already in use
```

**Fix:** Change `AGNIVO_HTTP_PORT` or stop the conflicting process.

---

## Health Check Failures

### `/health/ready` returns 503

Check which dependency failed:

```bash
curl -s http://localhost:9090/health/ready | jq .
```

Common causes:
- PostgreSQL unreachable — verify `AGNIVO_DATABASE_URL`
- Redis unreachable — verify `AGNIVO_REDIS_URL`
- Connection pool exhausted — increase `AGNIVO_DATABASE_MAX_CONNS`

### Docker healthcheck failing

Ensure the probe hits the **admin port** (9090), not the public port:

```yaml
healthcheck:
  test: ["CMD", "wget", "-qO-", "http://127.0.0.1:9090/health/live"]
```

---

## Authentication Issues

### 401 on all API requests

- Verify `Authorization: Bearer <token>` header is present
- Check token expiry (default 15 minutes for access tokens)
- Use `/auth/refresh` to get a new access token

### 401 on internal APIs in production

**Cause:** `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN` not set or token mismatch.

**Fix:** Set the same token on all services that communicate internally.

### 403 on org/project routes

**Cause:** User lacks required RBAC permission for the action.

**Fix:** Check member role in the organization. Owners and admins have full access.

---

## Deployment Issues

### Deployment stuck in `building`

1. Check builder worker is running: `make run-builder`
2. Check job queue: `SELECT * FROM jobs WHERE queue='builds' AND status='pending'`
3. Check builder logs for the deployment's `correlation_id`

### Deployment stuck in `deploying`

1. Verify scheduler is running and reachable at `AGNIVO_DEPLOYER_SCHEDULER_URL`
2. Verify runtime-agent is running and registered
3. Check deployer internal API: `GET /internal/v1/deployer/deployments/{id}`

### Build failures

Common causes:
- Git repository not connected or credentials expired
- Dockerfile detection failed — add explicit Dockerfile
- BuildKit/Docker not available on builder host

---

## Database Issues

### Connection pool timeout

```
timeout: context deadline exceeded
```

**Fix:** Increase pool size or reduce concurrent workers:

```bash
AGNIVO_DATABASE_MAX_CONNS=50
AGNIVO_BUILDER_CONCURRENCY=4
```

### Migration failure

Migrations run at module init. Check logs for the failing version (e.g. `0004_builder`).

**Fix:** Inspect the migration SQL in the module's `schema.go`. Apply manually if needed, then restart.

---

## Job Queue Issues

### Jobs not processing

```sql
SELECT queue, type, status, count(*) FROM jobs GROUP BY 1,2,3;
```

- `pending` jobs with no worker → start the appropriate executable
- `running` with expired lease → auto-reclaimed on next poll
- `dead` → inspect `last_error`, fix root cause, re-enqueue if needed

### Duplicate job execution

Should not happen — jobs use `FOR UPDATE SKIP LOCKED` with leases. If it does:
- Check lease duration vs job processing time
- Enable heartbeating for long-running jobs

---

## Proxy / SSL Issues

### Custom domain not routing

1. Check DNS verification: `GET /internal/v1/proxy/domains/{id}/verification`
2. Check route exists: `GET /internal/v1/proxy/routes/{hostname}`
3. Check Caddy connectivity from proxy-manager logs

### Certificate not issuing

- Verify DNS records are propagated
- Check `proxy_certificates` table for status and `last_error`
- Proxy reconciler retries automatically

---

## Observability

### No metrics on `/metrics`

- Metrics run on admin port 9090, not 8080
- If `AGNIVO_SECURITY_METRICS_BEARER_TOKEN` is set, include `Authorization: Bearer <token>`

### Missing traces

- Verify `AGNIVO_OBSERVABILITY_TRACING_ENABLED=true`
- Check OTLP endpoint reachability
- Sampling may drop traces — set `AGNIVO_OBSERVABILITY_TRACING_SAMPLER=1.0` for debugging

### Log correlation

Filter logs by request or correlation ID:

```json
{"correlation_id": "abc-123", "level": "error"}
```

---

## Performance

### High API latency

1. Check database query times in logs
2. Verify connection pool is not exhausted
3. Check Redis latency if rate limiting is enabled
4. Review Prometheus `agnivo_http_request_duration_seconds`

### High memory usage

- Builder workspace accumulation — check `AGNIVO_BUILDER_WORKSPACE_DIR` disk usage
- Connection pool too large — reduce `max_conns`
- Job queue backlog — scale worker instances

---

## Getting Help

1. Collect logs with correlation ID
2. Check `/health/ready` on all services
3. Inspect relevant database tables
4. See [Recovery Guide](recovery.md) for incident procedures
