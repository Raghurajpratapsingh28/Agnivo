.PHONY: build test test-race test-cover vet tidy wire lint generate run-% build-%

APPS := api builder deployer scheduler worker runtime-agent proxy-manager cron
GOFLAGS ?=

## build: compile every executable into bin/
build:
	@for app in $(APPS); do \
		echo "building $$app..."; \
		go build -o bin/$$app ./apps/$$app/cmd/$$app || exit 1; \
	done

## build-<app>: compile a single executable
build-%:
	go build -o bin/$* ./apps/$*/cmd/$*

## run-<app>: run a single executable
run-%:
	go run ./apps/$*/cmd/$*

## test: run all unit tests
test:
	go test $(GOFLAGS) ./...

## test-race: run tests with the race detector
test-race:
	go test -race $(GOFLAGS) ./...

## test-cover: run tests with coverage report
test-cover:
	go test -coverprofile=coverage.out $(GOFLAGS) ./...
	go tool cover -func=coverage.out | tail -1

## vet: static analysis
vet:
	go vet ./...

## lint: run golangci-lint (install: go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest)
lint:
	golangci-lint run ./...

## tidy: sync go.mod/go.sum
tidy:
	go mod tidy

## wire: regenerate Wire injectors (requires `go install github.com/google/wire/cmd/wire@latest`)
wire:
	cd packages/platform/bootstrap && wire
