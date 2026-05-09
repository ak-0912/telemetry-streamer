.PHONY: build test test-coverage proto run run-csv-streamer vet lint precommit-checks

APP_CMD := ./cmd/telemetry-streamer
CSV_STREAMER_CMD := ./cmd/csv-streamer
BUF_VERSION ?= v1.50.0

build: vet lint

test:
	go test ./...

test-coverage:
	go test ./... -coverprofile=coverage.out
	go tool cover -func=coverage.out

proto:
	go run github.com/bufbuild/buf/cmd/buf@$(BUF_VERSION) generate

run:
	go run $(APP_CMD)

# Example: make run-csv-streamer ARGS='-addr telemetry-message-queue:50051 -csv /path/to/dcgm_metrics_20250718_134233.csv -topic gpu-telemetry'
# Or set env: QUEUE_BACKEND=grpc MQ_GRPC_ADDR=host.docker.internal:50051 MQ_TOPIC=gpu-telemetry then: make run
run-csv-streamer:
	go run $(CSV_STREAMER_CMD) $(ARGS)

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
