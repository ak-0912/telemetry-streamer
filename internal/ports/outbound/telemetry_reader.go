package outbound

import (
	"context"

	"telemetry-streamer/internal/domain/telemetry"
)

// TelemetryReader abstracts the source of telemetry input (CSV, stream, etc.).
// Implementations must be safe for concurrent use across multiple workers.
type TelemetryReader interface {
	// Read starts producing telemetry readings on the returned channel.
	// The error channel carries fatal read errors; when either channel closes the reader is done.
	// Cancelling ctx stops production and closes both channels.
	Read(ctx context.Context) (<-chan telemetry.Reading, <-chan error)
}
