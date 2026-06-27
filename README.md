# Agnivo

AgentCloud — a production-grade cloud deployment platform built as a Go modular monolith with eight executables.

## Documentation

**Full documentation:** [docs/README.md](docs/README.md)

| Guide | Link |
|-------|------|
| Architecture | [docs/architecture.md](docs/architecture.md) |
| Configuration | [docs/configuration.md](docs/configuration.md) |
| Deployment | [docs/deployment.md](docs/deployment.md) |
| Public API | [docs/api/public-api.md](docs/api/public-api.md) |
| Development | [docs/development.md](docs/development.md) |
| Production Checklist | [docs/production-checklist.md](docs/production-checklist.md) |

## Structure

```
agnivo/
├── apps/              # 8 Go executables (api, builder, deployer, …)
├── packages/
│   ├── platform/      # Shared infrastructure (config, httpx, jobs, …)
│   └── application/   # Domain modules (identity, controlplane, build, …)
├── docs/              # Complete documentation
├── configs/           # Environment YAML overlays
├── docker/            # Dockerfiles
├── Makefile           # Build, test, lint
└── go.mod             # Single Go module
```

## Quick Start

```bash
cp .env.example .env
make build
AGNIVO_HTTP_ENABLED=true make run-api
```

Health: `http://localhost:9090/health/live`  
API: `http://localhost:8080/api/v1`

## Executables

| Binary | Role |
|--------|------|
| `api` | Public HTTP (dashboard, webhooks, streaming) |
| `builder` | Container image builds |
| `deployer` | Deployment orchestration |
| `scheduler` | Server placement and capacity |
| `runtime-agent` | Docker container lifecycle |
| `proxy-manager` | Edge routing, SSL, domains |
| `worker` | Background jobs (billing, GC, notifications) |
| `cron` | Distributed schedule firing |

See [apps/README.md](apps/README.md) for backend details.

## Docker

```bash
cp .env.example .env
docker compose up -d
```

API health probe: `GET http://localhost:9090/health/live`

## Scripts

| Command | Description |
|---------|-------------|
| `make build` | Compile all 8 executables → `bin/` |
| `make test` | Run all tests |
| `make test-race` | Tests with race detector |
| `make lint` | golangci-lint |
| `make run-api` | Run the API server |

## Frontend

Set `NEXT_PUBLIC_API_URL=http://localhost:8080` for the dashboard client.

Integration guide: [apps/api/docs/frontend-integration.md](apps/api/docs/frontend-integration.md)
