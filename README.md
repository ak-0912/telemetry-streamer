# telemetry-streamer
Scalable service that reads GPU telemetry from CSV and reliably publishes telemetry messages to the custom message queue to stimulate real-time telemetry streams

## Project structure (DDD + Clean Architecture)

- `cmd/telemetry-streamer`: application entrypoint and dependency wiring
- `internal/domain`: pure domain entities and business rules
- `internal/application`: use-cases orchestrating domain + ports
- `internal/ports`: inbound/outbound interfaces
- `internal/adapters`: infrastructure implementations for ports (CSV, queue, etc.)
- `internal/infrastructure`: config and setup concerns

## Implementation details

### End-to-end flow

1. `cmd/telemetry-streamer/main.go` loads runtime config and wires dependencies through Uber Fx.
2. CSV adapter (`internal/adapters/outbound/csv/reader.go`) loads all rows once.
3. Reader emits rows in an infinite loop (wraps to index `0` at EOF).
4. Each row is converted into a domain `Reading` with processing-time timestamp (`time.Now().UTC()`).
5. Use case validates and publishes telemetry over Connect RPC (`enqueue` endpoint) after adaptive delay.
6. External custom queue service accepts RPC requests and returns queue health signals.

### Services and responsibilities

- Producer service: `cmd/telemetry-streamer`
  - reads CSV loop
  - converts telemetry into protobuf messages (`structpb.Struct`)
  - sends through Connect RPC client to queue service
- DI layer:
  - telemetry producer is composed with Uber Fx providers + lifecycle hooks

### CSV ingestion and transformation

- CSV schema is read from `dcgm_metrics_20250718_134233.csv`.
- Required fields: `metric_name`, `gpu_id`, `device`, `uuid`, `modelName`, `Hostname`, `value`, `labels_raw`.
- Domain CSV model: `internal/domain/telemetry/entity.go` (`CSVRecord`).
- Transformation: `CSVRecord -> Reading` via `FromCSVRecord(...)`.
- Timestamp is assigned at processing time, not from CSV.

### Protobuf message schema

- Schema file: `api/telemetry.proto`.
- Message: `TelemetryMessage`.
- Encoded fields include:
  - metric metadata (`metric_name`, `gpu_id`, `device`, `uuid`, `model_name`, `host_name`)
  - metric value (`double value`)
  - raw labels (`labels_raw`)
  - processing timestamp (`processed_at_unix_nano`)
- Producer sends protobuf payload using Connect RPC.

### Backpressure handling

- Queue health is monitored via queue `Health` RPC (`depth`, `capacity`, `utilization`).
- Adaptive throttling behavior:
  - low utilization: base stream interval
  - medium/high utilization: interval multiplier (2x/4x/8x)
  - critical overload: publish may reject with overload signal
- Exponential backoff is applied on overload errors.
- Backoff decays smoothly after successful publishes.

### Worker model

- Streamer workers are managed in `internal/application/usecase/stream_telemetry.go`.
- Configurable fixed worker count per pod (`STREAM_WORKERS`).
- In-process worker scaling is intentionally disabled.
- Horizontal scaling is expected to be handled by Kubernetes.
- Worker instances are loosely coupled and stateless in behavior.
- Duplicate-tolerant behavior is expected at the queue/consumer side when multiple replicas publish similar samples.

### Kubernetes operations support

- Liveness probe endpoint: `GET /livez`
- Readiness probe endpoint: `GET /readyz`
- Metrics endpoint: `GET /metrics`
- Streamer replicas are dynamically scaled via Kubernetes HPA/KEDA; the application remains fixed-worker per pod with adaptive backpressure.

## Run

```bash
go run ./cmd/telemetry-streamer
```

## Config (env vars)

- `CSV_PATH` (default: `dcgm_metrics_20250718_134233.csv`)
- `STREAM_INTERVAL_MS` (default: `50`)
- `PROBE_ADDR` (default: `:8080`)
- `QUEUE_SERVICE_URL` (default: `http://127.0.0.1:8081`)
- `QUEUE_CAPACITY` (default: `1024`)
- `QUEUE_HIGH_WATERMARK` (default: `0.80`)
- `QUEUE_CRITICAL_WATERMARK` (default: `0.95`)
- `QUEUE_CONSUME_DELAY_MS` (default: `75`)
- `STREAM_WORKERS` (default: `1`)

## Message Schema

Protocol Buffer schema is defined in `api/telemetry.proto`.

## Local verification

```bash
go mod tidy
go build ./...
```

## Docker deployment

Build and tag the image:

```bash
docker build -t telemetry-streamer:latest .
```

## Kubernetes deployment with Helm

Chart location: `deploy/helm/telemetry-streamer`

Install/upgrade:

```bash
helm upgrade --install telemetry-streamer ./deploy/helm/telemetry-streamer \
  --namespace telemetry --create-namespace \
  --set image.repository=your-registry/telemetry-streamer \
  --set image.tag=latest
```

Render manifests only (dry check):

```bash
helm template telemetry-streamer ./deploy/helm/telemetry-streamer
```

## Local queue stub (for integration without real queue)

Run a minimal Connect-compatible queue stub:

```bash
go run ./cmd/queue-stub
```

Then run streamer against it:

```bash
QUEUE_SERVICE_URL=http://127.0.0.1:8081 go run ./cmd/telemetry-streamer
```

Stub env vars:

- `STUB_ADDR` (default: `:8081`)
- `STUB_CAPACITY` (default: `1024`)
- `STUB_REJECT_UTILIZATION` (default: `0.95`)
- `STUB_CONSUME_INTERVAL_MS` (default: `75`)
- `STUB_FAIL_ENQUEUE_PCT` (default: `0`)
