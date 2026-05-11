// Package observability provides lightweight, lock-free application metrics
// exposed via a Prometheus-compatible /metrics text endpoint.
package observability

import (
	"fmt"
	"net/http"
	"sync/atomic"
)

type streamMetrics struct {
	publishedTotal         atomic.Uint64
	publishErrorTotal      atomic.Uint64
	validationErrorTotal   atomic.Uint64
	readerErrorTotal       atomic.Uint64
	lastPublishLatencyNano atomic.Int64
	queueUtilizationMilli  atomic.Uint64
}

var metrics streamMetrics

// IncPublished increments the counter of successfully published messages (telemetry_published_total).
func IncPublished() {
	metrics.publishedTotal.Add(1)
}

// IncPublishError increments the counter of failed publish attempts (telemetry_publish_errors_total).
func IncPublishError() {
	metrics.publishErrorTotal.Add(1)
}

// IncValidationError increments the counter of readings rejected by domain validation (telemetry_validation_errors_total).
func IncValidationError() {
	metrics.validationErrorTotal.Add(1)
}

// IncReaderError increments the counter of reader-level errors (telemetry_reader_errors_total).
func IncReaderError() {
	metrics.readerErrorTotal.Add(1)
}

// ObservePublishLatencyNanos records the latest publish round-trip latency in nanoseconds
// (exposed as telemetry_last_publish_latency_seconds after conversion).
func ObservePublishLatencyNanos(v int64) {
	metrics.lastPublishLatencyNano.Store(v)
}

// ObserveQueueUtilization stores the latest queue utilization ratio (0.0–1.0)
// (exposed as telemetry_queue_utilization_ratio).
func ObserveQueueUtilization(utilization float64) {
	if utilization < 0 {
		utilization = 0
	}
	milli := uint64(utilization * 1000)
	metrics.queueUtilizationMilli.Store(milli)
}

// MetricsHandler returns an http.Handler that serves Prometheus text-format metrics at /metrics.
func MetricsHandler() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/plain; version=0.0.4")

		published := metrics.publishedTotal.Load()
		publishErrors := metrics.publishErrorTotal.Load()
		validationErrors := metrics.validationErrorTotal.Load()
		readerErrors := metrics.readerErrorTotal.Load()
		lastLatency := metrics.lastPublishLatencyNano.Load()
		queueUtilMilli := metrics.queueUtilizationMilli.Load()

		_, _ = fmt.Fprintf(w, "telemetry_published_total %d\n", published)
		_, _ = fmt.Fprintf(w, "telemetry_publish_errors_total %d\n", publishErrors)
		_, _ = fmt.Fprintf(w, "telemetry_validation_errors_total %d\n", validationErrors)
		_, _ = fmt.Fprintf(w, "telemetry_reader_errors_total %d\n", readerErrors)
		_, _ = fmt.Fprintf(w, "telemetry_last_publish_latency_seconds %.9f\n", float64(lastLatency)/1e9)
		_, _ = fmt.Fprintf(w, "telemetry_queue_utilization_ratio %.3f\n", float64(queueUtilMilli)/1000)
	})
}
