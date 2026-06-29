# syntax=docker/dockerfile:1
# Hot-reload dev image — mounts source at /src, uses `go run` for fast iteration.

FROM golang:1.25-alpine

RUN apk add --no-cache git ca-certificates

WORKDIR /src

COPY go.mod go.sum ./
RUN go mod download

COPY . .

EXPOSE 8080 9090

CMD ["go", "run", "./apps/api/cmd/api"]
