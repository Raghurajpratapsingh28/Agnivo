# syntax=docker/dockerfile:1

FROM golang:1.23-alpine

RUN apk add --no-cache git ca-certificates

WORKDIR /src/apps/backend

EXPOSE 8080

CMD ["go", "run", "./cmd/server"]
