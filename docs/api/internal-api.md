# Internal API Reference

Internal HTTP APIs are exposed by worker executables on dedicated ports. They are **not** publicly accessible — bind to private network interfaces and protect with `AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN`.

## Authentication

All internal APIs require:

```
Authorization: Bearer <AGNIVO_SECURITY_INTERNAL_SERVICE_TOKEN>
```

In production, requests without a valid token are rejected. In development, unset tokens allow unauthenticated access (with a startup warning).

## Shared Middleware

All internal servers use `App.RegisterInternalServer()` which applies:

- Request ID + Correlation ID
- Panic recovery
- Structured logging
- Request body limit (10 MiB default)
- Bearer token authentication

---

## Builder — port 8082

**Prefix:** `/internal/v1/builder`  
**Executable:** `builder`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/status` | Worker status and concurrency |
| GET | `/builds/{deploymentID}` | Build record for deployment |
| GET | `/builds/{deploymentID}/logs` | Build log output |
| POST | `/builds/{deploymentID}/cancel` | Cancel in-progress build |
| GET | `/workers` | Active worker instances |
| GET | `/queue` | Queue depth and pending jobs |

---

## Deployer — port 8083

**Prefix:** `/internal/v1/deployer`  
**Executable:** `deployer`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Service health |
| GET | `/ready` | Readiness probe |
| GET | `/live` | Liveness probe |
| GET | `/status` | Worker status |
| GET | `/deployments/{deploymentID}` | Deployment execution state |
| GET | `/deployments/{deploymentID}/phases` | Deployment phase timeline |
| GET | `/deployments/{deploymentID}/health` | Health check results |
| POST | `/deployments/{deploymentID}/cancel` | Cancel deployment |
| GET | `/metrics/summary` | Deploy metrics summary |
| GET | `/workers` | Active workers |
| GET | `/queue` | Queue depth |

---

## Scheduler — port 8084

**Prefix:** `/internal/v1`  
**Executable:** `scheduler`

| Method | Path | Description |
|--------|------|-------------|
| POST | `/reserve` | Reserve server capacity |
| POST | `/release` | Release reservation |
| POST | `/placement` | Request placement decision |
| POST | `/heartbeat` | Server heartbeat |
| GET | `/capacity` | Total platform capacity |
| GET | `/servers` | Registered servers |
| GET | `/health` | Health check |
| GET | `/ready` | Readiness |
| GET | `/live` | Liveness |
| GET | `/metrics/summary` | Scheduler metrics |

---

## Runtime Agent — port 8085

**Prefix:** `/internal/v1/runtime`  
**Executable:** `runtime-agent`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Agent health |
| GET | `/ready` | Readiness |
| GET | `/live` | Liveness |
| GET | `/diagnostics` | Node diagnostics |
| GET | `/containers` | List containers |
| POST | `/containers` | Create container |
| GET | `/containers/{containerID}` | Container details |
| POST | `/containers/{containerID}/start` | Start container |
| POST | `/containers/{containerID}/stop` | Stop container |
| POST | `/containers/{containerID}/restart` | Restart container |
| DELETE | `/containers/{containerID}` | Remove container |
| GET | `/containers/{containerID}/health` | Container health |
| GET | `/containers/{containerID}/logs` | Container logs |

---

## Proxy Manager — port 8086

**Prefix:** `/internal/v1/proxy`  
**Executable:** `proxy-manager`

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health`, `/ready`, `/live` | Health probes |
| POST | `/routes` | Create route |
| GET | `/routes` | List routes |
| GET | `/routes/{hostname}` | Get route |
| DELETE | `/routes/{hostname}` | Delete route |
| POST | `/traffic/switch` | Switch traffic |
| POST | `/blue-green` | Blue-green switch |
| POST | `/canary` | Canary deployment |
| POST | `/rollback` | Rollback route |
| POST | `/previews` | Create preview |
| GET | `/previews` | List previews |
| DELETE | `/previews/{deployment_id}` | Delete preview |
| GET | `/certs/{hostname}` | Certificate status |
| POST | `/certs/{hostname}/renew` | Force renewal |
| GET | `/domains/{domain_id}/verification` | DNS verification status |
| GET | `/stats` | Proxy statistics |
| GET | `/streaming/stats` | SSE connection stats |
| POST | `/jobs/domain-verify` | Trigger DNS verify job |
| POST | `/jobs/ssl-request` | Trigger SSL request job |

---

## API Host Internal Routes — port 8080

The `api` executable also exposes placement stubs at `/internal/v1` (protected by service token). In production deployments, use the dedicated `scheduler` executable instead.

| Method | Path | Description |
|--------|------|-------------|
| POST | `/internal/v1/placement` | Placement request (stub) |
| POST | `/internal/v1/reserve` | Reserve capacity (stub) |
| POST | `/internal/v1/release` | Release reservation (stub) |
| GET | `/internal/v1/capacity` | Capacity query (stub) |

---

## Service Discovery

In Docker Compose / Kubernetes, reference internal services by hostname:

```bash
AGNIVO_DEPLOYER_SCHEDULER_URL=http://scheduler:8084
AGNIVO_DEPLOYER_RUNTIME_AGENT_URL=http://runtime-agent:8085
AGNIVO_RUNTIME_AGENT_SCHEDULER_URL=http://scheduler:8084
```

Always include the bearer token in inter-service HTTP calls.
