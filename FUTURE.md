# Future Enhancements — Telemetry Streamer

---

## 1. External CSV via PVC

Mount the CSV from a PersistentVolumeClaim instead of baking it into the Docker image. Enables updating datasets without rebuilding. The Helm chart already has `csv.enabled` / `csv.existingPvc` conditionals ready — just needs a PVC manifest and documentation.

## 2. Graceful Drain on Shutdown

Wait for in-flight publishes to complete before exiting on SIGTERM. Currently the context is cancelled immediately, which may drop messages mid-flight.

## 3. CI Pipeline

Add a GitHub Actions workflow: lint → test → build image → push to registry. Currently all steps are manual.

## 4. Structured JSON Logs

Add a `LOG_FORMAT=json` option for production log aggregation (ELK, Loki, CloudWatch). The `log/slog` migration makes this a one-line handler swap.

## 5. Prometheus Histogram for Publish Latency

Replace the single "last latency" gauge with a histogram exposing p50/p95/p99 percentiles for meaningful SLO tracking.

## 6. Finite Replay Mode

Add a `--once` flag to stream the dataset exactly once and exit. Useful for batch ingestion, integration tests, and CI validation.

## 7. Periodic Pause / Scheduled Windows

Allow configurable quiet periods (e.g., pause streaming during maintenance windows) via cron-like schedule or admin signal.
