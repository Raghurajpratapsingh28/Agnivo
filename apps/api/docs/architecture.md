# Agnivo API — Unified HTTP Server

Single deployable binary for all client-facing and internal HTTP surfaces.

> **Documentation:** [docs/api/public-api.md](../../docs/api/public-api.md) · [OpenAPI](../../docs/api/openapi.yaml)  
> **Frontend developers:** [frontend-integration.md](./frontend-integration.md)

## Route map

```
/health/live
/health/ready
/webhooks/{provider}
/api/v1/...          → dashboard control plane
/cli/v1/...          → CLI & CI/CD
/stream/v1/...       → SSE & WebSocket
/internal/v1/...     → scheduler placement (same-host workers)
```

## Layout

```
internal/
├── routes/
│   ├── router.go       # mounts all surfaces
│   ├── v1/             # control plane
│   ├── cli/v1/         # CLI backend
│   ├── realtime/v1/    # streaming
│   ├── internal/v1/    # placement API
│   └── webhooks/
├── middleware/         # auth per surface
├── streaming/          # SSE/WS hub
├── handlers/
├── services/           # thin wiring to packages/application
└── bootstrap/
```

## Run

```bash
API_HTTP_PORT=8080 go run ./cmd/api
```
