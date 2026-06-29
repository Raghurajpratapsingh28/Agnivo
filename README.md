# Agnivo - 

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

## Deploy (single host, over SSH)

Deploy the full platform to a fresh Linux server with one command from your laptop.
It copies the repo, installs Docker if missing, generates a production `.env` with
strong secrets (internal token, metrics token, control-plane encryption key, and a
persistent JWT RS256 keypair), builds the images, and starts the stack.

```bash
# From your machine — pushes the working tree and deploys remotely:
./scripts/ssh-deploy.sh -i ~/.ssh/id_ed25519 --domain agnivo.example.com ubuntu@SERVER_IP

# Include the Next.js dashboard:
./scripts/ssh-deploy.sh --with-web --domain agnivo.example.com root@SERVER_IP

# Deploy from a git remote instead of rsync:
./scripts/ssh-deploy.sh --git https://github.com/you/agnivo.git deploy@SERVER_IP
```

Already on the server (in the repo root)? Run the bootstrap directly:

```bash
./scripts/deploy.sh --domain agnivo.example.com --with-web
./scripts/deploy.sh --down       # stop (keeps data volumes)
./scripts/deploy.sh --destroy    # stop + delete volumes (DATA LOSS)
```

Generated secrets live in `.env` on the server and are never overwritten on re-runs.
Persistent JWT keys require Docker Compose v2.24+ (otherwise the API falls back to
ephemeral keys and logs a warning).

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
