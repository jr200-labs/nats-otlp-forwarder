export GOOS ?= $(shell go env GOOS)
export GOARCH ?= $(shell go env GOARCH)

# Version is maintained by release-please in .release-please-manifest.json.
# Never edit the manifest by hand - push conventional commits and let
# release-please open a PR.
VERSION := $(shell sed -n 's/.*"\.": *"\([^"]*\)".*/\1/p' .release-please-manifest.json)

.DEFAULT_GOAL := all

DEV_REGISTRY ?= localhost:5005
DEV_IMAGE ?= $(DEV_REGISTRY)/jr200-labs/nats-otlp-forwarder:dev

.PHONY: all fmt test test-integration test-race view-coverage lint build clean docker-build-dev docker-push-dev

all: fmt lint build

fmt:
	go fmt ./...

test:
	go test -timeout=10m ./...

test-integration:
	go test -tags=integration -timeout=5m -count=1 ./tests/integration/...

test-race:
	go test -race -cover -coverprofile=coverage.out -timeout=10m ./...

view-coverage: test-race
	go tool cover -html=coverage.out

sync-shared-lint:
	@mkdir -p .shared
	@curl -sfL "https://raw.githubusercontent.com/jr200-labs/github-action-templates/master/shared/sync-shared-lint.sh" -o .shared/sync-shared-lint.sh
	@chmod +x .shared/sync-shared-lint.sh
	@./.shared/sync-shared-lint.sh go

lint: sync-shared-lint
	go vet ./...
	golangci-lint run --config .shared/.golangci.yml --timeout=5m

build:
	go mod tidy
	go mod download
	CGO_ENABLED=0 GOOS=$(GOOS) GOARCH=$(GOARCH) \
	go build -o build/nats-otlp-forwarder-$(GOOS)-$(GOARCH) -ldflags '-extldflags "-static" -X main.version=v$(VERSION)' \
	./cmd/nats-otlp-forwarder/

clean:
	@echo "Cleaning build artifacts and test cache..."
	rm -rf ./build
	rm -f coverage.out
	go clean -testcache

docker-build-dev:
	docker build \
		--build-arg BUILD_OS=linux \
		--build-arg BUILD_ARCH=$(GOARCH) \
		-f docker/Dockerfile \
		-t $(DEV_IMAGE) \
		.

docker-push-dev: docker-build-dev
	docker push $(DEV_IMAGE)

# NOTE: releases are fully automated via release-please - see
# .github/workflows/release-please.yaml. Do not add bump/release targets
# here; they will drift from the CI flow.
