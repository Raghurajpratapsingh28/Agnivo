# Agnivo Documentation

Complete documentation for the AgentCloud backend (v1.0.0).

## Getting Started

| Document | Description |
|----------|-------------|
| [Development Guide](development.md) | Local setup, testing, adding executables |
| [Configuration Guide](configuration.md) | All config structs, env vars, YAML overlays |
| [Deployment Guide](deployment.md) | Build, Docker, multi-instance production deploy |

## Architecture

| Document | Description |
|----------|-------------|
| [Architecture Overview](architecture.md) | System design, executables, data flow |
| [Module Reference](modules.md) | All `packages/application` modules |
| [Platform Packages](packages.md) | All `packages/platform` packages |
| [Architecture Validation](architecture-validation.md) | Per-executable production review |

## API Reference

| Document | Description |
|----------|-------------|
| [Public API](api/public-api.md) | Dashboard `/api/v1` endpoints |
| [Internal API](api/internal-api.md) | Service-to-service internal HTTP APIs |
| [OpenAPI Spec](api/openapi.yaml) | Machine-readable API definition |
| [Frontend Integration](../apps/api/docs/frontend-integration.md) | Next.js client guide with TypeScript examples |
| [API Server Layout](../apps/api/docs/architecture.md) | Route map and server structure |

## Operations

| Document | Description |
|----------|-------------|
| [Operations Guide](operations.md) | Health, metrics, tracing, cron, jobs |
| [Troubleshooting Guide](troubleshooting.md) | Common issues and diagnostics |
| [Recovery Guide](recovery.md) | Incident recovery runbooks |
| [Production Checklist](production-checklist.md) | Pre-launch verification |

## Engineering

| Document | Description |
|----------|-------------|
| [Security Guide](security.md) | Auth, secrets, OWASP, audit |
| [Coding Standards](coding-standards.md) | Go conventions and patterns |
| [Contribution Guide](contribution.md) | How to contribute |
| [Dependency Guide](dependencies.md) | Go module dependencies |
| [Migration Guide](migrations.md) | Database schema migrations |
| [Upgrade Guide](upgrade.md) | Version upgrades and rollbacks |

## Quick Links

```bash
make build          # compile all 8 executables
make test           # run all tests
make test-race      # race detector
make lint           # golangci-lint
cp .env.example .env
AGNIVO_HTTP_ENABLED=true make run-api
```

Health: `GET http://localhost:9090/health/live`  
Metrics: `GET http://localhost:9090/metrics`  
API: `http://localhost:8080/api/v1`
