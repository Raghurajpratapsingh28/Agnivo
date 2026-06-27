# Upgrade Guide

Procedures for upgrading Agnivo between versions.

## General Upgrade Process

1. Read the release notes for breaking changes
2. Run migrations in staging first
3. Deploy new binaries with rolling restart
4. Verify health checks and smoke tests
5. Monitor metrics and logs for 30 minutes

## Version Numbering

Agnivo follows semantic versioning: `MAJOR.MINOR.PATCH`

- **MAJOR** — breaking API or schema changes
- **MINOR** — new features, backward-compatible
- **PATCH** — bug fixes, no schema changes

## Pre-Upgrade Checklist

- [ ] Database backup completed (ops backup or manual `pg_dump`)
- [ ] Current version health checks passing
- [ ] Staging environment tested with new version
- [ ] Migration SQL reviewed
- [ ] Config changes documented (new env vars)
- [ ] Rollback plan prepared

## Upgrading Executables

All executables share the same Go module. Build and deploy all binaries together:

```bash
make build
# Deploy bin/api, bin/builder, bin/deployer, etc.
```

**Order matters for zero-downtime:**

1. `scheduler` — placement must be available before deploys
2. `runtime-agent` — nodes must register before new deploys
3. `builder`, `deployer` — workers can restart independently
4. `proxy-manager` — edge routing (brief DNS cache during restart)
5. `worker`, `cron` — background ops (safe to restart)
6. `api` — public API last (rolling restart behind load balancer)

## Database Migrations

Migrations run automatically on boot. When upgrading:

1. Deploy one instance of any executable that runs the full migration chain
2. Wait for migration to complete (check logs for `migration applied`)
3. Deploy remaining instances

If a migration fails, the executable will not start. Fix the SQL and redeploy — never amend an already-applied migration version.

See [Migration Guide](migrations.md) for details.

## Configuration Changes

When new config fields are added:

1. Check `.env.example` for new variables
2. Check `configs/production.yaml` for new defaults
3. Set new required variables before deploying

Breaking config changes are documented in release notes.

## Rolling Back

### Application rollback

1. Deploy previous binary versions from your artifact store
2. Restart executables in reverse order (api first, scheduler last)
3. Verify health checks

### Schema rollback

Schema migrations are forward-only. If a migration must be reversed:

1. Restore database from pre-upgrade backup
2. Deploy previous application version

**Do not** manually delete rows from `schema_migrations` unless you know exactly which migration to revert.

## Zero-Downtime Deployment

For production with multiple API instances:

1. Deploy new binaries to a subset of instances
2. Wait for `/health/ready` to pass
3. Shift load balancer traffic to new instances
4. Repeat for remaining instances
5. Deploy workers independently (job queue handles mixed versions briefly)

Job handlers must remain backward-compatible during rolling worker upgrades.

## Monitoring After Upgrade

Watch these metrics for 30 minutes post-upgrade:

| Metric | Expected |
|--------|----------|
| `agnivo_http_requests_total{status="5xx"}` | No spike |
| `/health/ready` | 200 on all services |
| Job queue depth | Draining normally |
| Error log rate | No increase |

## Version-Specific Notes

### v1.0.0 (Initial Release)

- First production release
- 9 database migrations (0001–0009)
- Required production env vars:
  - `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN`
  - `AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM` / `PUBLIC_KEY_PEM`
  - `AGNIVO_CONTROLPLANE_ENCRYPTION_KEY`
- Admin health probes on port 9090 (`/health/live`, `/health/ready`)
- Docker image builds from `apps/api/cmd/api`

See [Production Checklist](production-checklist.md) for full launch requirements.
