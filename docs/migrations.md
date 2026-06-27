# Migration Guide

Database schema changes are managed through versioned migrations in each application module.

## Migration Chain

Migrations are applied automatically at module initialization via `postgres.DB.Migrate()`. Each module chains from its upstream module:

```
0001_identity        → identity tables
0002_controlplane    → projects, deployments, env vars, secrets, domains
0003_jobs            → platform job queue table
0004_builder         → build records, logs, artifacts
0005_deployer        → deploy executions, phases, health checks
0006_scheduler       → servers, reservations, capacity
0007_runtimeagent    → containers, node state
0008_proxy           → routes, certificates, domain verifications, previews
0009_ops             → billing, metering, quotas, notifications, audit, analytics
```

The full chain runs when the last module in the chain initializes (typically `ops` via worker, or `proxy` via proxy-manager).

## How Migrations Work

Each module defines:

```go
// packages/application/<module>/schema.go
func Migrations() []postgres.Migration {
    m := upstream.Migrations()  // chain from previous module
    m = append(m, postgres.Migration{
        Version: "0004_builder",
        SQL:     SchemaDDL,
    })
    return m
}
```

Migrations are tracked in a `schema_migrations` table. Already-applied versions are skipped.

## Adding a New Migration

1. **Never modify applied migrations** — always add a new version
2. Write idempotent SQL (`IF NOT EXISTS`, `IF EXISTS`)
3. Chain from the latest module or add as a new version in the same module
4. Test against a fresh database and an existing database

Example — adding a column:

```go
const migration0010 = `
ALTER TABLE ops_subscriptions ADD COLUMN IF NOT EXISTS metadata JSONB DEFAULT '{}'::jsonb;
`

func Migrations() []postgres.Migration {
    m := proxy.Migrations()
    m = append(m, postgres.Migration{Version: "0010_ops_metadata", SQL: migration0010})
    return m
}
```

## Running Migrations Manually

Migrations run automatically on boot. To inspect:

```sql
SELECT * FROM schema_migrations ORDER BY version;
```

To apply on a fresh database, start any executable that initializes the full chain (e.g. `worker` or `api` with all modules).

## Rollback Strategy

Migrations are **forward-only**. To rollback:

1. Deploy the previous application version (which expects the previous schema)
2. If a migration added columns/tables that the old version doesn't use, they are harmless
3. If a migration removed or renamed columns, restore from backup before deploying the old version

**Always test migrations in staging before production.**

## Data Migrations

For data transformations (not just DDL):

1. Add the new schema in a DDL migration
2. Write a one-time job or startup hook to backfill data
3. Remove old columns in a subsequent migration after backfill completes

## Index Guidelines

- Add indexes for foreign keys and hot query paths
- Use partial indexes for filtered queries (`WHERE status='pending'`)
- Name indexes: `<table>_<columns>_idx`
- Include indexes in the same migration as the table creation

See `packages/platform/jobs/schema.go` for index examples on the job queue.

## Local Development

```bash
# Start PostgreSQL
docker run -d --name agnivo-pg -e POSTGRES_PASSWORD=agnivo -p 5432:5432 postgres:15

# Configure
AGNIVO_DATABASE_ENABLED=true
AGNIVO_DATABASE_URL=postgres://postgres:agnivo@localhost:5432/postgres?sslmode=disable

# Migrations run on first boot
make run-api
```

## Troubleshooting

| Issue | Fix |
|-------|-----|
| Migration version conflict | Check `schema_migrations` table for applied versions |
| DDL syntax error | Fix SQL, increment version number, redeploy |
| Slow migration on large table | Use `CREATE INDEX CONCURRENTLY` in production |
| Missing upstream tables | Ensure migration chain order is correct |

See [Troubleshooting Guide](troubleshooting.md) for more diagnostics.
