#!/usr/bin/env sh

set -eu

BASE_URL="${BASE_URL:-http://127.0.0.1:8080}"

pass() {
  echo "PASS: $1"
}

fail() {
  echo "FAIL: $1"
  exit 1
}

body="$(curl -fsS "${BASE_URL}/livez" || true)"
[ "$body" = "ok" ] || fail "/livez expected 'ok', got '${body}'"
pass "/livez is healthy"

body="$(curl -fsS "${BASE_URL}/readyz" || true)"
[ "$body" = "ready" ] || fail "/readyz expected 'ready', got '${body}'"
pass "/readyz is ready"

metrics1="$(curl -fsS "${BASE_URL}/metrics")"
published1="$(printf "%s\n" "$metrics1" | awk '/^telemetry_published_total / {print $2}')"
errors1="$(printf "%s\n" "$metrics1" | awk '/^telemetry_publish_errors_total / {print $2}')"
util1="$(printf "%s\n" "$metrics1" | awk '/^telemetry_queue_utilization_ratio / {print $2}')"

[ -n "${published1:-}" ] || fail "telemetry_published_total not found in /metrics"
[ -n "${errors1:-}" ] || fail "telemetry_publish_errors_total not found in /metrics"
[ -n "${util1:-}" ] || fail "telemetry_queue_utilization_ratio not found in /metrics"
pass "metrics keys are present"

sleep 3

metrics2="$(curl -fsS "${BASE_URL}/metrics")"
published2="$(printf "%s\n" "$metrics2" | awk '/^telemetry_published_total / {print $2}')"
errors2="$(printf "%s\n" "$metrics2" | awk '/^telemetry_publish_errors_total / {print $2}')"

[ "$published2" -gt "$published1" ] || fail "published counter did not increase (${published1} -> ${published2})"
pass "published counter is increasing (${published1} -> ${published2})"

echo "INFO: publish errors (${errors1} -> ${errors2})"
echo "INFO: queue utilization ratio ${util1}"
echo "All checks passed."
