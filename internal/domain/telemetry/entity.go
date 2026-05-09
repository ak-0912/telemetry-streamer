package telemetry

import "time"

// CSVRecord represents one parsed CSV line before normalization.
type CSVRecord struct {
	MetricName string
	GPUId      string
	Device     string
	UUID       string
	ModelName  string
	HostName   string
	Value      float64
	LabelsRaw  string
}

// Reading represents a single telemetry event from a GPU source.
type Reading struct {
	MetricName string
	GPUId      string
	Device     string
	UUID       string
	ModelName  string
	HostName   string
	Value      float64
	LabelsRaw  string
	// Timestamp is the instant this log was processed; it is the canonical telemetry time (not source-clock or CSV wall time).
	Timestamp time.Time
}

// FromCSVRecord stamps processing time and converts into domain reading.
func FromCSVRecord(record CSVRecord, processedAt time.Time) Reading {
	return Reading{
		MetricName: record.MetricName,
		GPUId:      record.GPUId,
		Device:     record.Device,
		UUID:       record.UUID,
		ModelName:  record.ModelName,
		HostName:   record.HostName,
		Value:      record.Value,
		LabelsRaw:  record.LabelsRaw,
		Timestamp:  processedAt,
	}
}
