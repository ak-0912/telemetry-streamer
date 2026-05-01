#!/usr/bin/env sh

set -eu

QUEUE_URL="${QUEUE_SERVICE_URL:-http://127.0.0.1:8081}"
QUEUE_ADDR="${STUB_ADDR:-:8081}"

cleanup() {
  if [ -n "${STREAMER_PID:-}" ] && kill -0 "$STREAMER_PID" 2>/dev/null; then
    kill "$STREAMER_PID" 2>/dev/null || true
  fi
  if [ -n "${STUB_PID:-}" ] && kill -0 "$STUB_PID" 2>/dev/null; then
    kill "$STUB_PID" 2>/dev/null || true
  fi
}

trap cleanup INT TERM EXIT

echo "Starting queue stub on ${QUEUE_ADDR}..."
STUB_ADDR="$QUEUE_ADDR" go run ./cmd/queue-stub >/tmp/queue-stub.log 2>&1 &
STUB_PID=$!

sleep 1

echo "Starting telemetry streamer (QUEUE_SERVICE_URL=${QUEUE_URL})..."
QUEUE_SERVICE_URL="$QUEUE_URL" go run ./cmd/telemetry-streamer >/tmp/telemetry-streamer.log 2>&1 &
STREAMER_PID=$!

echo "queue-stub pid: ${STUB_PID} (logs: /tmp/queue-stub.log)"
echo "streamer  pid: ${STREAMER_PID} (logs: /tmp/telemetry-streamer.log)"
echo "Press Ctrl+C to stop both."

wait "$STREAMER_PID"
