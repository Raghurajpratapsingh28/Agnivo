# Recovery Guide

## Database Failure

**Symptoms:** `/health/ready` returns 503 for `postgres`; all writes fail.

**Recovery:**
1. Verify PostgreSQL is running and reachable
2. Check connection pool exhaustion: `SELECT count(*) FROM pg_stat_activity WHERE datname='agnivo'`
3. Restore from latest ops backup if data corruption detected
4. Restart affected executables after DB recovery

## Redis Failure

**Symptoms:** Rate limiting degraded; cron leader election fails; streaming fan-out broken.

**Recovery:**
1. Redis is not required for core API CRUD — api remains functional
2. Restart Redis; executables reconnect automatically
3. Cron may double-fire briefly during leader transition — jobs are idempotent

## Job Queue Stuck

**Symptoms:** Jobs in `running` state with expired leases.

**Recovery:**
1. Leases auto-reclaim on expiry — no manual intervention needed
2. For permanently stuck jobs: `UPDATE jobs SET status='dead' WHERE id='...'`
3. Inspect `last_error` column for root cause

## Certificate Expiry

**Symptoms:** HTTPS errors on custom domains.

**Recovery:**
1. Proxy-manager reconciler auto-renews certs 30 days before expiry
2. Check `proxy_certificates` table for status
3. Trigger manual renewal via proxy internal API

## Deployment Failure

**Symptoms:** Deployment stuck in `deploying` state.

**Recovery:**
1. Check deployer worker logs by `correlation_id`
2. Verify scheduler placement and runtime-agent health
3. Rollback via controlplane API or re-trigger deploy job

## Full Platform Recovery

1. Restore PostgreSQL from backup
2. Start Redis
3. Start executables in order: scheduler → runtime-agent → builder → deployer → proxy-manager → worker → cron → api
4. Verify `/health/ready` on each admin port
5. Run proxy reconciler to heal route drift

## Backup Restore Validation

```bash
# Verify latest backup exists
SELECT id, kind, status, storage_path, completed_at FROM ops_backups
WHERE status='completed' ORDER BY completed_at DESC LIMIT 1;
```

Test restore in an isolated staging environment before production recovery.
