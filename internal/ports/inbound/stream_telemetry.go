package inbound

import (
	"context"

	"telemetry-streamer/internal/domain/telemetry"
)

// StreamTelemetryUseCase is the entrypoint contract for starting telemetry flow.
type StreamTelemetryUseCase interface {
	Stream(ctx context.Context) error
	Handle(ctx context.Context, reading telemetry.Reading) error
}
