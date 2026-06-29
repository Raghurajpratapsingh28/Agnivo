# syntax=docker/dockerfile:1
# Parameterised Agnivo service image.
#
# The expensive `build` stage compiles ALL executables exactly once and is
# shared (by content hash) across every service image, so `docker compose build`
# no longer recompiles the module 8 times. Each final image just copies the one
# binary it needs, selected with --build-arg APP=<name>.
#
#   docker build --build-arg APP=builder -f docker/agnivo.Dockerfile .

# ── compile (shared by all services) ──────────────────────────────────────────
FROM golang:1.25-alpine AS build

RUN apk add --no-cache git ca-certificates

WORKDIR /src

# Module download is cached as a layer AND in a BuildKit cache mount, so it runs
# at most once and is reused on subsequent builds.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
# Build every binary in one pass. The shared Go build cache means packages are
# compiled once and reused across all 8 executables instead of 8 cold compiles.
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    set -eux; \
    for app in api builder deployer scheduler worker runtime-agent proxy-manager cron; do \
      CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
        go build -ldflags="-s -w" -o "/out/${app}" "./apps/${app}/cmd/${app}"; \
    done

# ── runtime ──────────────────────────────────────────────────────────────────
FROM alpine:3.21

ARG APP=api

# docker-cli lets builder + runtime-agent talk to the mounted Docker socket.
RUN apk add --no-cache ca-certificates tzdata wget docker-cli

COPY --from=build /out/${APP} /app
COPY --from=build /src/configs /configs

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:9090/health/live || exit 1

CMD ["/app"]
