package outbound

import (
	"context"

	"telemetry-streamer/internal/domain/telemetry"
)

// TelemetryReader abstracts the source of telemetry input (CSV, stream, etc.).
type TelemetryReader interface {
	Read(ctx context.Context) (<-chan telemetry.Reading, <-chan error)
}
