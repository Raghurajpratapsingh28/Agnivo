package dockerfile

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/Raghurajpratapsingh28/Agnivo/packages/application/build/detect"
)

const generatorVersion = "agnivo-dfgen-1.0"

// GenerateResult describes Dockerfile generation output.
type GenerateResult struct {
	Path      string
	Content   string
	Version   string
	Generated bool
}

// Generator produces optimized Dockerfiles when none exist.
type Generator struct{}

// NewGenerator constructs a Dockerfile generator.
func NewGenerator() *Generator { return &Generator{} }

// Generate writes or validates a Dockerfile. Custom Dockerfiles always take precedence.
func (g *Generator) Generate(workspaceDir string, fw detect.Framework) (GenerateResult, error) {
	custom := filepath.Join(workspaceDir, "Dockerfile")
	if data, err := os.ReadFile(custom); err == nil && len(strings.TrimSpace(string(data))) > 0 {
		return GenerateResult{Path: custom, Content: string(data), Version: "custom", Generated: false}, nil
	}
	content := g.template(fw)
	path := custom
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		return GenerateResult{}, err
	}
	return GenerateResult{Path: path, Content: content, Version: generatorVersion, Generated: true}, nil
}

func (g *Generator) template(fw detect.Framework) string {
	labels := fmt.Sprintf(`LABEL org.opencontainers.image.source="agnivo-builder"
LABEL org.opencontainers.image.vendor="agnivo"
LABEL org.opencontainers.image.title="%s"`, fw.Name)

	switch fw.Name {
	case "nextjs":
		return g.nodeMultiStage(fw, "npm ci", "npm run build", "npm start", "3000", labels)
	case "remix", "react", "express", "fastify", "nestjs", "nodejs":
		return g.nodeMultiStage(fw, "npm ci", "npm run build || true", "node server.js || npm start", "3000", labels)
	case "fastapi", "flask", "django", "python":
		return g.pythonMultiStage(fw, labels)
	case "go":
		return g.goMultiStage(labels)
	case "rust":
		return g.rustMultiStage(labels)
	case "spring-boot", "java":
		return g.javaMultiStage(labels)
	case "ruby":
		return g.rubyMultiStage(labels)
	case "php":
		return g.phpMultiStage(labels)
	case "static":
		return g.staticNginx(labels)
	default:
		return g.generic(labels)
	}
}

func (g *Generator) nodeMultiStage(fw detect.Framework, install, build, start, port, labels string) string {
	runtime := fw.Runtime
	if runtime == "" {
		runtime = "node20"
	}
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM node:%s-alpine AS deps
WORKDIR /app
COPY package*.json ./
RUN %s

FROM node:%s-alpine AS builder
WORKDIR /app
COPY --from=deps /app/node_modules ./node_modules
COPY . .
RUN %s

FROM node:%s-alpine AS runner
WORKDIR /app
ENV NODE_ENV=production
RUN addgroup -g 1001 -S agnivo && adduser -S agnivo -u 1001 -G agnivo
COPY --from=builder --chown=agnivo:agnivo /app .
USER agnivo
EXPOSE %s
%s
HEALTHCHECK --interval=30s --timeout=3s --start-period=10s CMD wget -qO- http://127.0.0.1:%s/ || exit 1
CMD %s
`, strings.TrimPrefix(runtime, "node"), install,
		strings.TrimPrefix(runtime, "node"), build,
		strings.TrimPrefix(runtime, "node"), port, labels, port, start)
}

func (g *Generator) pythonMultiStage(fw detect.Framework, labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM python:3.12-slim AS builder
WORKDIR /app
RUN apt-get update && apt-get install -y --no-install-recommends build-essential && rm -rf /var/lib/apt/lists/*
COPY requirements.txt pyproject.toml* ./
RUN pip install --no-cache-dir -r requirements.txt 2>/dev/null || pip install --no-cache-dir .

FROM gcr.io/distroless/python3-debian12 AS runner
WORKDIR /app
COPY --from=builder /usr/local/lib/python3.12/site-packages /usr/local/lib/python3.12/site-packages
COPY . .
USER 65532:65532
EXPOSE 8000
%s
CMD ["python", "-m", "uvicorn", "main:app", "--host", "0.0.0.0", "--port", "8000"]
`, labels)
}

func (g *Generator) goMultiStage(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM golang:1.22-alpine AS builder
WORKDIR /src
RUN apk add --no-cache git ca-certificates
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/app .

FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=builder /out/app /app
USER nonroot:nonroot
EXPOSE 8080
%s
ENTRYPOINT ["/app"]
`, labels)
}

func (g *Generator) rustMultiStage(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM rust:1.78-alpine AS builder
WORKDIR /src
RUN apk add --no-cache musl-dev
COPY Cargo.toml Cargo.lock ./
COPY src ./src
RUN cargo build --release

FROM alpine:3.20
RUN adduser -D -u 1001 agnivo
COPY --from=builder /src/target/release/* /app/
USER agnivo
EXPOSE 8080
%s
ENTRYPOINT ["/app/main"]
`, labels)
}

func (g *Generator) javaMultiStage(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM eclipse-temurin:21-jdk-alpine AS builder
WORKDIR /src
COPY . .
RUN ./mvnw -q package -DskipTests || ./gradlew bootJar -x test

FROM gcr.io/distroless/java21-debian12:nonroot
COPY --from=builder /src/target/*.jar /app/app.jar
USER nonroot:nonroot
EXPOSE 8080
%s
ENTRYPOINT ["java", "-jar", "/app/app.jar"]
`, labels)
}

func (g *Generator) rubyMultiStage(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM ruby:3.3-alpine AS builder
WORKDIR /app
RUN apk add --no-cache build-base
COPY Gemfile Gemfile.lock ./
RUN bundle config set --local deployment 'true' && bundle install
COPY . .

FROM ruby:3.3-alpine AS runner
WORKDIR /app
RUN adduser -D -u 1001 agnivo
COPY --from=builder /app /app
USER agnivo
EXPOSE 3000
%s
CMD ["bundle", "exec", "rails", "server", "-b", "0.0.0.0"]
`, labels)
}

func (g *Generator) phpMultiStage(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM composer:2 AS builder
WORKDIR /app
COPY composer.json composer.lock ./
RUN composer install --no-dev --optimize-autoloader
COPY . .

FROM php:8.3-fpm-alpine AS runner
WORKDIR /app
RUN adduser -D -u 1001 agnivo
COPY --from=builder /app /app
USER agnivo
EXPOSE 8080
%s
CMD ["php", "-S", "0.0.0.0:8080", "-t", "public"]
`, labels)
}

func (g *Generator) staticNginx(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM nginx:1.27-alpine
COPY . /usr/share/nginx/html
%s
HEALTHCHECK --interval=30s CMD wget -qO- http://127.0.0.1/ || exit 1
`, labels)
}

func (g *Generator) generic(labels string) string {
	return fmt.Sprintf(`# syntax=docker/dockerfile:1
FROM alpine:3.20
WORKDIR /app
COPY . .
RUN adduser -D -u 1001 agnivo && chown -R agnivo:agnivo /app
USER agnivo
EXPOSE 8080
%s
CMD ["sh", "-c", "echo 'Configure CMD in Dockerfile' && sleep infinity"]
`, labels)
}

// Version returns the generator version string.
func Version() string { return generatorVersion }
