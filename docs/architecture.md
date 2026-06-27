# Agnivo Platform Architecture

Agnivo (AgentCloud) is a modular monolith for cloud-native application deployment. Eight executables share a common platform layer and domain packages.

## Executables

| Binary | Role | HTTP |
|--------|------|------|
| `api` | Public dashboard, CLI, streaming, webhooks | Public :8080 + Admin :9090 |
| `builder` | Container image builds | Internal :8082 + Admin :9090 |
| `deployer` | Deployment orchestration | Internal :8083 + Admin :9090 |
| `scheduler` | Placement & capacity | Internal :8084 + Admin :9090 |
| `runtime-agent` | Docker lifecycle on nodes | Internal :8085 + Admin :9090 |
| `proxy-manager` | Edge routing, SSL, domains | Internal :8086 + Admin :9090 |
| `worker` | Background jobs (ops, billing, GC) | Admin :9090 only |
| `cron` | Distributed schedule firing | Admin :9090 only |

## Layering

```
apps/<executable>/cmd          Entry points
apps/<executable>/internal     Executable-specific wiring only
packages/platform              Shared infrastructure (no business logic)
packages/application           Domain modules (identity, controlplane, build, …)
```

**Rule:** Business logic lives in `packages/application`. Executables only wire modules via `bootstrap.Run` and `Register`.

## Bootstrap Lifecycle

Every executable uses `bootstrap.Run(name, register)`:

1. Load & validate config (Viper + validator)
2. Wire DI (logger, metrics, tracing, Postgres, Redis)
3. Start admin server (`/health/live`, `/health/ready`, `/metrics`)
4. Call executable `Register` (routes, workers, hooks)
5. Start public HTTP if enabled
6. Block until SIGINT/SIGTERM → graceful shutdown (reverse hooks, bounded timeout)

## Data Flow

```
Git push → api/webhooks → controlplane → jobs queue
                ↓
           builder (image) → deployer → scheduler (placement)
                ↓                              ↓
           runtime-agent (containers) ←─────────┘
                ↓
           proxy-manager (routes, SSL) → Internet
                ↓
           worker/cron (billing, GC, analytics, backups)
```

## Events & Jobs

- **Jobs:** PostgreSQL-backed queue with `SKIP LOCKED`, leases, idempotency, DLQ
- **Events:** In-process bus (swap for Redis/NATS later without changing producers)

## Extension Points (5-year horizon)

| Capability | Extension |
|------------|-----------|
| GPU scheduling | Scheduler algorithm + runtime-agent resource model |
| Multi-region | Scheduler regions + proxy-manager routing |
| Kubernetes | Runtime-agent backend interface |
| Enterprise SSO | Identity OIDC/SAML providers |
| Terraform | Public API + stable resource IDs |
| Custom builders | Builder executor registry |
| Edge regions | Proxy-manager multi-instance + geo DNS |

## Related Docs

- [Deployment Guide](deployment.md)
- [Operations Guide](operations.md)
- [Security](security.md)
- [Production Checklist](production-checklist.md)
- [Architecture Validation](architecture-validation.md)
