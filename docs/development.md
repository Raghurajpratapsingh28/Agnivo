# Development Guide

## Setup

```bash
cp .env.example .env
# Enable database/redis as needed
make build
make run-api
```

## Testing

```bash
make test           # all unit tests
make test-race      # race detector
make test-cover     # coverage summary
```

Integration tests require:

```bash
export DATABASE_TEST_URL=postgres://...
export REDIS_TEST_URL=redis://...
go test ./...
```

## Code Standards

- Business logic in `packages/application`, not in `apps/`
- Use `bootstrap.Run` for all executables
- Errors via `packages/platform/errors` (typed codes)
- Context propagation for all I/O
- Structured logging with `zap`
- Prometheus metrics for operational paths

## Adding a New Executable

1. Create `apps/<name>/cmd/<name>/main.go` calling `bootstrap.Run`
2. Create `apps/<name>/internal/app/app.go` with `Register` function
3. Add to `Makefile` APPS list
4. Add config struct to `packages/platform/config/config.go`

## Linting

```bash
make lint    # requires golangci-lint
make vet
```

## Wire DI

```bash
make wire    # regenerate bootstrap injectors
```

## Migrations

Add DDL to your module's `schema.go` and chain from the previous module's `Migrations()`.

## Related

- [Architecture](architecture.md)
- [API Route Map](../apps/api/docs/architecture.md)
