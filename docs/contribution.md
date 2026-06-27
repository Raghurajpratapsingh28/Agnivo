# Contribution Guide

Thank you for contributing to Agnivo. This guide covers the workflow for making changes to the backend.

## Prerequisites

- Go 1.25+
- PostgreSQL and Redis (for integration tests)
- golangci-lint (`go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest`)

## Getting Started

```bash
git clone <repo>
cd agnivo
cp .env.example .env
make build
make test
```

See [Development Guide](development.md) for full setup.

## Branch Strategy

- `main` — stable, production-ready
- Feature branches: `feat/<description>`, `fix/<description>`, `docs/<description>`

## Making Changes

### 1. Understand the architecture

- Business logic goes in `packages/application/<module>/`
- Shared infrastructure goes in `packages/platform/`
- Executables in `apps/<name>/` only wire modules — no business logic

Read [Architecture](architecture.md) and [Coding Standards](coding-standards.md) before coding.

### 2. Write tests

- Unit tests alongside the code (`*_test.go`)
- Use `testkit` for fakes and fixtures
- Integration tests require `DATABASE_TEST_URL` and `REDIS_TEST_URL`

```bash
make test
make test-race    # always run before submitting
```

### 3. Lint and vet

```bash
make vet
make lint
```

### 4. Keep changes focused

- One concern per PR
- No unrelated refactors
- Match existing naming, error handling, and import style

## Pull Request Checklist

- [ ] `make test` passes
- [ ] `make test-race` passes
- [ ] `make vet` passes
- [ ] `make lint` passes (if golangci-lint installed)
- [ ] New code has tests for non-trivial behavior
- [ ] Documentation updated if APIs or config changed
- [ ] No secrets committed
- [ ] Migration added if schema changed (see [Migration Guide](migrations.md))

## Code Review Expectations

Reviewers check for:

- Correct module boundaries (no cross-module internal imports)
- Context propagation on all I/O
- Typed errors from `packages/platform/errors`
- Idempotent job handlers and reconciliation loops
- Security: no hardcoded secrets, input validation, auth on new routes

## Adding a New Module

1. Create `packages/application/<name>/` with `module.go`, `schema.go`, `model/`, `store/`
2. Chain migrations from the upstream module
3. Wire in the appropriate executable's `Register` function
4. Add documentation to [Module Reference](modules.md)

## Adding a New Executable

1. Create `apps/<name>/cmd/<name>/main.go`
2. Create `apps/<name>/internal/app/app.go` with `Register`
3. Add config struct to `packages/platform/config/config.go`
4. Add to `Makefile` APPS list
5. Document in [Architecture](architecture.md)

## Reporting Issues

Include:
- Executable and version
- Relevant config (redact secrets)
- Log output with correlation ID
- Steps to reproduce

## License

By contributing, you agree that your contributions will be licensed under the project's license.
