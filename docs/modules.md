# Module Reference

All domain logic lives in `packages/application/`. Each module has a `module.go` composition root and is initialized by one or more executables.

## identity

**Path:** `packages/application/identity`  
**Initialized by:** `api`  
**Migration:** `0001_identity`

Authentication, authorization, organizations, and user management.

| Subpackage | Responsibility |
|------------|----------------|
| `auth` | Login, register, refresh, password reset |
| `user` | User profiles |
| `organization` | Org CRUD |
| `member` | Org membership and invitations |
| `session` | Session lifecycle |
| `apikey` | API key management |
| `pat` | Personal access tokens |
| `jwt` | RS256 token issuance and validation |
| `rbac` | Role-based permissions |
| `tenant` | Request-scoped org/user context |
| `password` | bcrypt hashing |
| `tokencrypto` | Refresh token encryption |
| `audit` | Identity audit events |
| `http` | REST handlers and middleware |

**Does not own:** deployments, billing, containers.

---

## controlplane

**Path:** `packages/application/controlplane`  
**Initialized by:** `api`  
**Migration:** `0002_controlplane`, `0003_jobs`

Projects, deployments, environment variables, secrets, domains, and git webhooks.

| Subpackage | Responsibility |
|------------|----------------|
| `project` | Project lifecycle |
| `deployment` | Deployment records and state machine |
| `envvar` | Encrypted environment variables |
| `secret` | Encrypted secrets with rotation |
| `domain` | Custom domain management |
| `gitrepo` | Git repository connections |
| `webhook` | GitHub/GitLab/Bitbucket webhook ingestion |
| `cpjobs` | Job enqueuing (build, deploy, sleep, wake) |
| `cpevents` | Control plane event publishing |
| `crypto` | AES-GCM encryption for secrets |
| `http` | REST handlers |

**Does not own:** container execution, routing, billing.

---

## build

**Path:** `packages/application/build`  
**Initialized by:** `builder` executable  
**Migration:** `0004_builder`

Container image builds from git repositories.

| Subpackage | Responsibility |
|------------|----------------|
| `executor` | Build orchestration pipeline |
| `buildkit` | BuildKit/Docker build integration |
| `dockerfile` | Dockerfile generation |
| `detect` | Framework/runtime detection |
| `git` | Repository cloning |
| `logs` | Build log streaming |
| `cache` | Layer cache management |
| `ecr` | AWS ECR push |
| `sbom` | Software bill of materials |
| `cancel` | Build cancellation |
| `worker` | Job queue consumer |
| `http` | Internal status API |

**Consumes:** `builds` queue, job type `build.run`

---

## deploy

**Path:** `packages/application/deploy`  
**Initialized by:** `deployer` executable  
**Migration:** `0005_deployer`

Deployment orchestration: scheduling, container creation, health checks, rollback.

| Subpackage | Responsibility |
|------------|----------------|
| `executor` | Deploy pipeline orchestration |
| `strategy` | Rolling, blue-green, canary strategies |
| `health` | Startup/readiness/liveness probes |
| `runtime` | Runtime agent client |
| `scheduler` | Scheduler client for placement |
| `rollback` | Deployment rollback |
| `recovery` | Failed deployment recovery |
| `secrets` | Secret injection at deploy time |
| `ecr` | Image pull from ECR |
| `cancel` | Deploy cancellation |
| `worker` | Job queue consumer |
| `http` | Internal status API |

**Consumes:** `deployments` queue — `deploy.run`, `deploy.rollback`, `deploy.delete`, `project.sleep`, `project.wake`

---

## scheduler

**Path:** `packages/application/scheduler`  
**Initialized by:** `scheduler` executable  
**Migration:** `0006_scheduler`

Server placement, capacity tracking, reservations, and load balancing.

| Subpackage | Responsibility |
|------------|----------------|
| `engine` | Placement algorithms (least_loaded, etc.) |
| `placement` | Reservation and release logic |
| `store` | Server capacity persistence |
| `events` | Scheduler event publishing |
| `http` | Internal placement API |

**Does not own:** container execution (delegates to runtime-agent).

---

## runtimeagent

**Path:** `packages/application/runtimeagent`  
**Initialized by:** `runtime-agent` executable  
**Migration:** `0007_runtimeagent`

Docker container lifecycle on a single host node.

| Subpackage | Responsibility |
|------------|----------------|
| `docker` | Docker Engine API client |
| `executor` | Start, stop, restart, remove containers |
| `health` | Container health monitoring |
| `heartbeat` | Capacity reporting to scheduler |
| `recovery` | Orphan container reconciliation |
| `store` | Local container state |
| `http` | Internal container API |

**Does not own:** scheduling decisions, routing, builds.

---

## proxy

**Path:** `packages/application/proxy`  
**Initialized by:** `proxy-manager` executable  
**Migration:** `0008_proxy`

Edge networking: Caddy routing, SSL, DNS verification, traffic switching, previews.

| Subpackage | Responsibility |
|------------|----------------|
| `caddy` | Caddy Admin API client |
| `route` | Dynamic route engine |
| `cert` | ACME certificate lifecycle |
| `dns` | Domain ownership verification |
| `traffic` | Blue-green, canary, rolling switches |
| `preview` | Ephemeral preview environments |
| `streaming` | Redis pub/sub SSE fan-out |
| `middleware` | Edge rate limiting, security headers |
| `recovery` | Route/cert reconciliation |
| `http` | Internal proxy API |

**Consumes:** `domains` queue — `domain.verify`, `domain.ssl_request`

---

## ops

**Path:** `packages/application/ops`  
**Initialized by:** `worker` and `cron` executables  
**Migration:** `0009_ops`

Platform operations: billing, metering, quotas, notifications, backups, GC, analytics, audit, auto-sleep.

| Subpackage | Responsibility |
|------------|----------------|
| `billing` | Plans, subscriptions, invoices, credits |
| `metering` | Usage tracking and rollups |
| `quota` | Plan limit enforcement |
| `notification` | Email, Slack, Discord, webhook delivery |
| `backup` | Database backup and retention |
| `cleanup` | Garbage collection of expired resources |
| `analytics` | Daily platform metrics aggregation |
| `audit` | Enterprise audit trail |
| `autosleep` | Idle project sleep/wake |
| `cron` | Distributed schedule firing |
| `jobs` | Ops job handlers |
| `events` | Ops event publishing |
| `metrics` | Prometheus collectors |

**Consumes:** `ops`, `notifications`, `backup`, `cleanup`, `analytics`, `billing`, `autosleep` queues

---

## Module Boundaries

```
api          → identity, controlplane
builder      → build (+ controlplane deps for git/env)
deployer     → deploy (+ build, controlplane, scheduler, runtime)
scheduler    → scheduler
runtime-agent → runtimeagent
proxy-manager → proxy (+ controlplane domains)
worker       → ops
cron         → ops (scheduler mode only)
```

No module imports another module's internal implementation — only shared interfaces, job payloads, and event types.
