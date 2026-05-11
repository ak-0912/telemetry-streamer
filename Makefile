.PHONY: build build-binaries check test test-coverage proto run vet lint precommit-checks

APP_CMD := ./cmd/telemetry-streamer
BIN_DIR := bin
BUF_VERSION ?= v1.50.0

# Compile only → ./bin/telemetry-streamer
build: build-binaries

# Vet + lint + compile (use in CI or before release)
check: vet lint build-binaries

build-binaries:
	mkdir -p $(BIN_DIR)
	go build -o $(BIN_DIR)/telemetry-streamer $(APP_CMD)

test:
	go test ./...

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

proto:
	go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

# Optional ARGS: -addr host:port -csv path -topic name (see README)
run:
	go run $(APP_CMD) $(ARGS)

vet:
	go vet ./...

lint:
	@if command -v golangci-lint >/dev/null 2>&1; then \
		golangci-lint run ./...; \
	else \
		echo "golangci-lint is not installed. Install from https://golangci-lint.run/welcome/install/"; \
		exit 1; \
	fi

precommit-checks:
	./scripts/precommit-checks.sh
