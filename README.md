# telemetry-streamer
Scalable service that reads GPU telemetry from CSV and reliably publishes telemetry messages to the custom message queue to stimulate real-time telemetry streams

## Project structure (DDD + Clean Architecture)

- `cmd/telemetry-streamer`: application entrypoint and dependency wiring
- `internal/domain`: pure domain entities and business rules
- `internal/application`: use-cases orchestrating domain + ports
- `internal/ports`: outbound interfaces (reader, publisher, queue monitor)
- `internal/adapters`: infrastructure implementations for ports (CSV, queue, etc.)
- `internal/infrastructure`: config and setup concerns

## Implementation details

### End-to-end flow

1. `cmd/telemetry-streamer/main.go` loads runtime config and wires dependencies through Uber Fx.
2. CSV adapter (`internal/adapters/outbound/csv/reader.go`) loads all rows once.
3. Reader emits rows in an infinite loop (wraps to index `0` at EOF).
4. Each row is converted into a domain `Reading` with processing-time timestamp (`time.Now().UTC()`).
5. Use case validates and publishes telemetry over **gRPC** (`mq.v1.MessageQueueService/Publish`) after adaptive delay.
6. The MQ has no health RPC; the publisher synthesizes **virtual queue depth** from publish outcomes for adaptive throttling (see `QUEUE_CAPACITY` / `QUEUE_CONSUME_DELAY_MS`).

### Services and responsibilities

- Producer service: `cmd/telemetry-streamer`
  - reads CSV loop
  - converts telemetry into protobuf `TelemetryMessage` (`api/telemetry.pb.go`)
  - sends through **`google.golang.org/grpc`** using `mq.v1.MessageQueueServiceClient`
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
- Producer sends **`telemetry.v1.TelemetryMessage`** bytes as `mq.v1.PublishRequest.payload` using `api/mq/v1/message_queue.proto`.

### Backpressure handling

- Queue pressure for throttling uses **synthetic** `depth` / `capacity` / `utilization` derived locally (aligned with `QUEUE_CAPACITY` and publish pacing).
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

### MQ over gRPC (env)

When the MQ listens on gRPC, set **`QUEUE_BACKEND=grpc`** and **`MQ_GRPC_ADDR`** (host:port or full `grpc://` / `grpcs://` URL). That **overrides** `QUEUE_SERVICE_URL` for the dial target used by the publisher.

```bash
export QUEUE_BACKEND=grpc
export MQ_GRPC_ADDR=host.docker.internal:50051   # from Linux Docker host → MQ often needs --add-host=host.docker.internal:host-gateway
export MQ_TOPIC=gpu-telemetry
# Consumer-side naming only — mq.v1 PublishRequest has topic/key/payload only.
# Use the same value in telemetry collectors that call Fetch/JoinGroup/etc.
export MQ_GROUP=telemetry-collector

make run
```

If **`rpc error: code = Unavailable`**: MQ is not reachable at `MQ_GRPC_ADDR` from where the streamer runs (wrong hostname for Docker/K8s, MQ bound only to `127.0.0.1`, wrong port, or MQ process down). From **inside a Linux container**, `host.docker.internal` may be unset unless you add `--add-host=host.docker.internal:host-gateway` (Compose/K8s) or dial the MQ **service DNS name** and published port instead.

### Optional CLI flags (`cmd/telemetry-streamer`)

Before Fx starts, non-empty flags call `os.Setenv` for:

- `-addr` → `QUEUE_BACKEND=grpc`, `MQ_GRPC_ADDR`
- `-csv` → `CSV_PATH`
- `-topic` → `MQ_TOPIC`

```bash
make run ARGS='-addr host.docker.internal:50051 -csv /workspaces/<repo>/dcgm_metrics_20250718_134233.csv -topic gpu-telemetry'
```

Set `export MQ_GROUP=telemetry-collector` separately if your tooling expects it (publish path ignores `MQ_GROUP`).

## Config (env vars)

- `CSV_PATH` (default: `dcgm_metrics_20250718_134233.csv`)
- `STREAM_INTERVAL_MS` (default: `50`)
- `PROBE_ADDR` (default: `:8080`)
- `QUEUE_BACKEND` (optional) — set to `grpc` with `MQ_GRPC_ADDR` to derive the gRPC dial URL
- `MQ_GRPC_ADDR` (optional) — e.g. `host.docker.internal:50051` or `grpc://telemetry-message-queue:50051`; used when `QUEUE_BACKEND=grpc`
- `QUEUE_SERVICE_URL` (default: `http://127.0.0.1:8081`; also supports `grpc://`, `grpcs://`, `https://`, or plain `host:port`) — ignored for dial when `QUEUE_BACKEND=grpc` and `MQ_GRPC_ADDR` are set
- `MQ_TOPIC` (default: `telemetry`) — `PublishRequest.topic`
- `MQ_GROUP` (optional, **not read by this repo**) — set only if your ops/collector stack expects it; consumer RPCs (`Fetch`, `JoinGroup`, …) use `group`; **`Publish` does not** send it
- `MQ_KEY_STRATEGY` (default: `gpu_id`) — one of: `static`, `gpu_id`, `metric_name`, `metric_gpu`, `uuid`
- `MQ_KEY_STATIC` (default: empty) — key when `MQ_KEY_STRATEGY=static`; if empty, key falls back to `MQ_TOPIC`
- `QUEUE_CAPACITY` (default: `1024`)
- `QUEUE_HIGH_WATERMARK` (default: `0.80`)
- `QUEUE_CRITICAL_WATERMARK` (default: `0.95`)
- `QUEUE_CONSUME_DELAY_MS` (default: `75`)
- `STREAM_WORKERS` (default: `1`)

## Message Schema

Protocol Buffer schema is defined in `api/telemetry.proto`.

Regenerate Go types from the schema (requires network on first run to fetch `buf` and remote plugins):

```bash
make proto
```

This writes `api/telemetry.pb.go` and `api/mq/v1/message_queue.pb.go` plus `api/mq/v1/message_queue_grpc.pb.go` using `buf.yaml` and `buf.gen.yaml`.

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

Run a minimal **gRPC** implementation of `mq.v1.MessageQueueService` (stub implements `Publish`):

```bash
go run ./cmd/queue-stub
```

Then run streamer against it (insecure gRPC; `http://` is accepted and dialled as plaintext gRPC):

```bash
QUEUE_SERVICE_URL=http://127.0.0.1:8081 go run ./cmd/telemetry-streamer
```

Or explicitly:

```bash
QUEUE_SERVICE_URL=grpc://127.0.0.1:8081 go run ./cmd/telemetry-streamer
```

Stub env vars:

- `STUB_ADDR` (default: `:8081`)
- `STUB_CAPACITY` (default: `1024`)
- `STUB_REJECT_UTILIZATION` (default: `0.95`)
- `STUB_CONSUME_INTERVAL_MS` (default: `75`)
- `STUB_FAIL_ENQUEUE_PCT` (default: `0`)
