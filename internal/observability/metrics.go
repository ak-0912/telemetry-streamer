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

func IncPublished() {
	metrics.publishedTotal.Add(1)
}

func IncPublishError() {
	metrics.publishErrorTotal.Add(1)
}

func IncValidationError() {
	metrics.validationErrorTotal.Add(1)
}

func IncReaderError() {
	metrics.readerErrorTotal.Add(1)
}

func ObservePublishLatencyNanos(v int64) {
	metrics.lastPublishLatencyNano.Store(v)
}

func ObserveQueueUtilization(utilization float64) {
	if utilization < 0 {
		utilization = 0
	}
	milli := uint64(utilization * 1000)
	metrics.queueUtilizationMilli.Store(milli)
}

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
