# syntax=docker/dockerfile:1
# Parameterised Agnivo service image.
# Build any of the 8 executables by passing --build-arg APP=<name>.
#
#   docker build --build-arg APP=builder -f docker/agnivo.Dockerfile .

# ── compile ──────────────────────────────────────────────────────────────────
FROM golang:1.25-alpine AS build

ARG APP=api

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

ARG TARGETOS=linux
ARG TARGETARCH=amd64
RUN CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -ldflags="-s -w" -o /agnivo ./apps/${APP}/cmd/${APP}

# ── runtime ──────────────────────────────────────────────────────────────────
FROM alpine:3.21

# docker-cli lets builder + runtime-agent talk to the mounted Docker socket.
RUN apk add --no-cache ca-certificates tzdata wget docker-cli

COPY --from=build /agnivo /app
COPY --from=build /src/configs /configs

EXPOSE 8080 9090

HEALTHCHECK --interval=30s --timeout=5s --start-period=20s --retries=3 \
    CMD wget -qO- http://127.0.0.1:9090/health/live || exit 1

CMD ["/app"]
