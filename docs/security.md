# Security Guide

## Authentication & Authorization

- **Dashboard API:** JWT RS256 access tokens + refresh tokens
- **API keys / PATs:** Bearer tokens with prefix `agn_` / `pat_`
- **Internal APIs:** Bearer service token (`AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN`)
- **RBAC:** Organization-scoped roles enforced via identity middleware

## Transport Security

- Terminate TLS at load balancer or Caddy
- HSTS enabled automatically in production when `X-Forwarded-Proto: https`
- Internal service ports should bind to private network only

## Secrets Management

| Secret | Env Var | Rotation |
|--------|---------|----------|
| JWT private key | `AGNIVO_IDENTITY_JWT_PRIVATE_KEY_PEM` | Key rotation with overlap period |
| Encryption key | `AGNIVO_CONTROLPLANE_ENCRYPTION_KEY` | Re-encrypt secrets on rotation |
| Service token | `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN` | Rolling deploy with new token |
| Webhook HMAC | `AGNIVO_CONTROLPLANE_WEBHOOKS_*_SECRET` | Per-provider rotation |
| Stripe | `AGNIVO_OPS_STRIPE_*` | Stripe dashboard |

Never commit secrets. Use a secrets manager (AWS SM, Vault, etc.) in production.

## HTTP Hardening

Public API middleware chain:

1. Request ID + Correlation ID
2. Panic recovery
3. Structured logging
4. Security headers (CSP, X-Frame-Options, COOP, CORP)
5. CORS (restrict origins in production)
6. Request body limit (default 10 MiB)
7. Per-request timeout

## CSRF

Cookie-based auth endpoints should use `identity/http.CSRFProtection(allowedOrigins)` matching CORS origins.

## Rate Limiting

- Edge proxy: Redis token bucket per client IP
- Identity: login rate limits (see identity module)
- API: configure at reverse proxy layer for DDoS protection

## Container Security

- Runtime agent runs containers with non-root users where possible
- Docker socket access limited to runtime-agent hosts
- Image verification: enable ECR signing in production (builder/deployer ECR config)

## Audit

All administrative actions are recorded in `ops_audit_events` with actor, resource, correlation ID, and change payload.

## OWASP Top 10 Mitigations

| Risk | Mitigation |
|------|------------|
| Injection | Parameterized queries (pgx), input validation |
| Broken Auth | JWT + RBAC + service tokens |
| Sensitive Data | Encrypted secrets (AES-GCM), TLS |
| XXE | JSON-only API, no XML parsing |
| Broken Access | Org-scoped RBAC middleware |
| Misconfiguration | Validated config, production overlay |
| XSS | CSP headers, JSON API (no HTML rendering) |
| Insecure Deserialization | JSON with size limits |
| Known Vulnerabilities | govulncheck in CI |
| Insufficient Logging | Structured audit + correlation IDs |
