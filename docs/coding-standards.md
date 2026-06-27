# Coding Standards

Conventions for Go code in the Agnivo backend.

## Architecture Rules

1. **Business logic in `packages/application`** — never in `apps/`
2. **Infrastructure in `packages/platform`** — no domain knowledge
3. **Executables are thin** — `bootstrap.Run(name, Register)` + route wiring only
4. **Module boundaries** — modules communicate via jobs, events, and HTTP APIs, not direct imports of internal packages

## Package Organization

```
packages/application/<module>/
├── module.go          # Composition root
├── schema.go          # DDL + Migrations()
├── model/             # Domain types
├── store/             # Database repository
├── http/              # HTTP handlers (if applicable)
└── <subsystem>/       # Business logic packages
```

## Naming

| Item | Convention | Example |
|------|-----------|---------|
| Packages | lowercase, single word | `billing`, `metering` |
| Files | snake_case for multi-word | `circuitbreaker.go` |
| Exported types | PascalCase | `Subscription`, `Engine` |
| Interfaces | noun or `-er` suffix | `BillingProvider`, `Sender` |
| Constants | PascalCase or grouped | `PlanFree`, `TypeSleep` |
| Env vars | `AGNIVO_<SECTION>_<KEY>` | `AGNIVO_HTTP_PORT` |
| DB tables | `<module>_<entity>` | `ops_subscriptions` |
| Job types | dot-separated | `build.run`, `ops.cleanup` |
| Event names | dot-separated | `ops.quota.exceeded` |

## Error Handling

Use typed errors from `packages/platform/errors`:

```go
return errors.New(errors.CodeNotFound, "project not found")
return errors.Wrap(err, errors.CodeInternal, "billing: create invoice")
return errors.Unauthenticated("invalid token")
```

Never return raw `fmt.Errorf` from handler or service boundaries.

## Context Propagation

Every function that performs I/O accepts `context.Context` as the first parameter:

```go
func (r *Repository) GetSubscription(ctx context.Context, orgID string) (model.Subscription, error)
```

Extract correlation IDs from context for logging:

```go
logger.From(ctx).Info("billing: invoice generated", zap.String("org_id", orgID))
```

## Database Access

- Use `postgres.DB.Conn(ctx)` for queries (participates in transactions)
- Translate pgx errors: `postgres.Translate(err, "ops: get subscription")`
- Use parameterized queries — never string-interpolate SQL
- Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`, `CREATE INDEX IF NOT EXISTS`)

## HTTP Handlers

- Decode with `dto.Decode(w, r, &req)`
- Respond with `httpx.OK(w, data)` or `dto.Error(w, r, err)`
- Register routes in module `http/router.go` or `http/handlers.go`
- Apply middleware at the router level, not inside handlers

## Jobs

- Enqueue with idempotency keys for at-least-once safety
- Handlers must be idempotent — safe to retry
- Use heartbeats for jobs exceeding the lease duration
- Return errors to trigger retry; return nil to complete

## Events

- Use dot-separated names: `deployment.created`, `ops.quota.exceeded`
- Include correlation ID, org ID, and actor in metadata
- Prefer async publish (`PublishAsync`) unless the caller needs delivery confirmation

## Logging

- Use structured fields (`zap.String`, `zap.Error`) — no string formatting in messages
- Log at appropriate levels: `Info` for operations, `Warn` for recoverable issues, `Error` for failures
- Never log secrets, tokens, or encryption keys

## Testing

- Table-driven tests for multiple input cases
- Use `testify/require` for fatal assertions, `assert` for non-fatal
- Mock external services via interfaces (e.g. `BillingProvider`, `Sender`)
- Integration tests skip when `DATABASE_TEST_URL` is unset

## Comments

- Package comment on every exported package
- Comment non-obvious business logic only
- No commented-out code in commits

## Imports

Group in order: standard library, third-party, platform, application:

```go
import (
    "context"
    "time"

    "github.com/go-chi/chi/v5"
    "go.uber.org/zap"

    "github.com/agnivo/agnivo/packages/platform/errors"
    "github.com/agnivo/agnivo/packages/application/ops/model"
)
```

## Security

- Validate all external input
- Use constant-time comparison for tokens (`crypto/subtle`)
- Encrypt secrets at rest (AES-GCM via controlplane crypto)
- Never expose private keys or encryption keys in API responses
- Apply auth middleware on every non-public route
