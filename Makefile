export GOOS ?= $(shell go env GOOS)
export GOARCH ?= $(shell go env GOARCH)

VERSION := $(shell grep '^version' pyproject.toml | head -1 | sed 's/.*"\(.*\)"/\1/')

.DEFAULT_GOAL := all

DEV_REGISTRY ?= localhost:5005
DEV_IMAGE ?= $(DEV_REGISTRY)/jr200-labs/nats-otlp-forwarder:dev

.PHONY: all fmt test test-integration test-race view-coverage lint build clean bump release docker-build-dev docker-push-dev

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

bump:
	@if [ -z "$(PART)" ]; then echo "Usage: make bump PART=major|minor|patch"; exit 1; fi
	@IFS='.' read -r major minor patch <<< "$(VERSION)"; \
	case "$(PART)" in \
		major) major=$$((major + 1)); minor=0; patch=0;; \
		minor) minor=$$((minor + 1)); patch=0;; \
		patch) patch=$$((patch + 1));; \
		*) echo "PART must be major, minor, or patch"; exit 1;; \
	esac; \
	new_version="$$major.$$minor.$$patch"; \
	sed -i '' "s/^version = \"$(VERSION)\"/version = \"$$new_version\"/" pyproject.toml; \
	echo "Bumped version: $(VERSION) -> $$new_version"

docker-build-dev:
	docker build \
		--build-arg BUILD_OS=linux \
		--build-arg BUILD_ARCH=$(GOARCH) \
		-f docker/Dockerfile \
		-t $(DEV_IMAGE) \
		.

docker-push-dev: docker-build-dev
	docker push $(DEV_IMAGE)

release: lint test test-integration
	@echo "Creating release v$(VERSION)..."
	git tag "v$(VERSION)"
	git push origin "v$(VERSION)"
	gh release create "v$(VERSION)" --generate-notes
