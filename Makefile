.PHONY: build test test-coverage run vet lint precommit-checks

APP_CMD := ./cmd/telemetry-streamer

build: vet lint

test:
	go test ./...

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

run:
	go run $(APP_CMD)

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
