# Dependency Guide

Agnivo uses a single Go module (`github.com/Raghurajpratapsingh28/Agnivo`) with Go 1.25.

## Direct Dependencies

| Package | Version | Purpose |
|---------|---------|---------|
| `go-chi/chi/v5` | v5.3.0 | HTTP router |
| `go-chi/cors` | v1.2.2 | CORS middleware |
| `jackc/pgx/v5` | v5.10.0 | PostgreSQL driver and connection pool |
| `redis/go-redis/v9` | v9.21.0 | Redis client |
| `spf13/viper` | v1.21.0 | Configuration loading |
| `go-playground/validator/v10` | v10.30.3 | Struct validation |
| `go.uber.org/zap` | v1.28.0 | Structured logging |
| `prometheus/client_golang` | v1.23.2 | Prometheus metrics |
| `go.opentelemetry.io/otel` | v1.44.0 | Distributed tracing |
| `golang-jwt/jwt/v5` | v5.3.1 | JWT RS256 tokens |
| `golang.org/x/crypto` | v0.52.0 | bcrypt, AES-GCM |
| `google/wire` | v0.7.0 | Compile-time dependency injection |
| `google/uuid` | v1.6.0 | UUID generation |
| `docker/docker` | v27.5.1 | Docker Engine API |
| `stretchr/testify` | v1.11.1 | Test assertions |
| `joho/godotenv` | v1.5.1 | .env file loading |
| `golang.org/x/sync` | v0.20.0 | errgroup, semaphore |

## Dependency Principles

1. **Minimal surface** — prefer standard library where possible
2. **No ORM** — raw SQL via pgx for performance and control
3. **No message broker** — PostgreSQL-native job queue
4. **No web framework** — chi router + platform httpx utilities
5. **Pinned versions** — all direct deps pinned in `go.mod`

## Updating Dependencies

```bash
# Check for updates
go list -m -u all

# Update a specific dependency
go get github.com/jackc/pgx/v5@latest
go mod tidy

# Verify nothing broke
make test
make vet
```

## Security Scanning

CI runs `govulncheck` on every push:

```bash
go install golang.org/x/vuln/cmd/govulncheck@latest
govulncheck ./...
```

Review and patch vulnerabilities before production releases.

## Docker Dependencies

| Image | Purpose |
|-------|---------|
| `golang:1.25-alpine` | Build stage |
| `alpine:3.21` | Runtime stage |
| `postgres:15` | Database (compose) |
| `redis:7` | Cache (compose) |

## External Services

| Service | Required | Purpose |
|---------|----------|---------|
| PostgreSQL 15+ | Yes (production) | Primary data store, job queue |
| Redis 7+ | Recommended | Locks, rate limits, pub/sub, cron election |
| Docker Engine | Yes (builder, deployer, runtime-agent) | Container builds and runtime |
| Caddy | Yes (proxy-manager) | Edge routing and SSL |
| OTLP Collector | Optional | Trace export |
| SMTP | Optional | Email notifications |
| Stripe | Optional | Billing (ops module) |

## License Compliance

All direct dependencies use permissive licenses (MIT, Apache 2.0, BSD). Run license audit before enterprise deployment if required by your compliance team.
