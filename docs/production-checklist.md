# Production Readiness Checklist

Use this checklist before launching AgentCloud v1.0.0.

## Configuration

- [ ] `AGNIVO_APP_ENVIRONMENT=production`
- [ ] All secrets from environment/secrets manager (not YAML files)
- [ ] `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN` set (64+ random bytes)
- [ ] `AGNIVO_SECURITY_METRICS_BEARER_TOKEN` set
- [ ] JWT keys loaded (not auto-generated)
- [ ] Control plane encryption key set
- [ ] CORS origins restricted to dashboard domain
- [ ] Database and Redis URLs use TLS (`sslmode=require` / `rediss://`)

## Infrastructure

- [ ] PostgreSQL: backups enabled, connection pooling sized
- [ ] Redis: persistence configured, memory limits set
- [ ] Docker: runtime-agent hosts hardened, socket access restricted
- [ ] Caddy: admin API not publicly exposed
- [ ] TLS certificates automated (Let's Encrypt via proxy-manager)
- [ ] Network: internal ports (8082–8086) on private network only

## Observability

- [ ] Structured JSON logging shipping to aggregator
- [ ] Prometheus scraping admin `/metrics` (with bearer token)
- [ ] OpenTelemetry traces exported (sampler ≤ 0.1)
- [ ] Alerting on: error rate, queue depth, cert expiry, disk usage
- [ ] Dashboards for DAU, deployments, MRR (ops analytics)

## Security

- [ ] Internal APIs require service token
- [ ] Stream routes require JWT authentication
- [ ] Webhook HMAC secrets configured per provider
- [ ] Rate limiting at edge (proxy-manager + load balancer)
- [ ] govulncheck passes in CI
- [ ] Audit logging enabled and retained (365 days default)

## Reliability

- [ ] Graceful shutdown tested (SIGTERM → drain → exit)
- [ ] Health probes configured (live + ready on :9090)
- [ ] Job queue DLQ monitored
- [ ] Backup schedule verified (daily DB backup)
- [ ] GC workers running (hourly cleanup)
- [ ] Cron leader election verified (Redis)

## Executables

| Service | Ready | Health | Metrics | Auth |
|---------|-------|--------|---------|------|
| api | ☐ | /health/ready | ☐ | JWT + RBAC |
| builder | ☐ | /health/ready | ☐ | Service token |
| deployer | ☐ | /health/ready | ☐ | Service token |
| scheduler | ☐ | /health/ready | ☐ | Service token |
| runtime-agent | ☐ | /health/ready | ☐ | Service token |
| proxy-manager | ☐ | /health/ready | ☐ | Service token |
| worker | ☐ | /health/ready | ☐ | N/A (no HTTP API) |
| cron | ☐ | /health/ready | ☐ | N/A (no HTTP API) |

## Disaster Recovery

- [ ] Database backup restore tested
- [ ] Runbook documented ([recovery.md](recovery.md))
- [ ] RTO/RPO defined and validated
- [ ] Incident response contacts defined

## CI/CD

- [ ] GitHub Actions CI passing (test, vet, lint, govulncheck)
- [ ] Docker images built for target architecture (amd64 + arm64)
- [ ] Migration validation in staging before production deploy
- [ ] Rollback procedure tested

## Sign-off

| Role | Name | Date |
|------|------|------|
| Engineering Lead | | |
| Security | | |
| Operations | | |
