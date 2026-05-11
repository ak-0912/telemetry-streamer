// Package telemetry contains the core domain model: entities, value objects,
// and business-rule validation for GPU telemetry data.
package telemetry

import "time"

// CSVRecord represents one parsed CSV line before normalization.
type CSVRecord struct {
	MetricName string  // DCGM metric name (e.g. "DCGM_FI_DEV_GPU_UTIL").
	GPUID      string  // Logical GPU index on the host (e.g. "0", "1").
	Device     string  // PCIe device identifier.
	UUID       string  // Globally unique GPU identifier assigned by NVIDIA.
	ModelName  string  // GPU model (e.g. "Tesla T4").
	Hostname   string  // Hostname of the machine reporting the metric.
	Value      float64 // Metric sample value.
	LabelsRaw  string  // Original Prometheus-style label string, preserved verbatim.
}

// Reading represents a single telemetry event from a GPU source.
type Reading struct {
	MetricName string  // DCGM metric name.
	GPUID      string  // Logical GPU index.
	Device     string  // PCIe device identifier.
	UUID       string  // Globally unique GPU identifier.
	ModelName  string  // GPU model name.
	Hostname   string  // Source host.
	Value      float64 // Metric sample value.
	LabelsRaw  string  // Original label string, preserved verbatim.
	// Timestamp is the instant this log was processed; it is the canonical telemetry time (not source-clock or CSV wall time).
	Timestamp time.Time
}

// FromCSVRecord stamps processing time and converts into domain reading.
func FromCSVRecord(record CSVRecord, processedAt time.Time) Reading {
	return Reading{
		MetricName: record.MetricName,
		GPUID:      record.GPUID,
		Device:     record.Device,
		UUID:       record.UUID,
		ModelName:  record.ModelName,
		Hostname:   record.Hostname,
		Value:      record.Value,
		LabelsRaw:  record.LabelsRaw,
		Timestamp:  processedAt,
	}
}
