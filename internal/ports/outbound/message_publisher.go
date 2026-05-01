package outbound

import (
	"context"

	"telemetry-streamer/internal/domain/telemetry"
)

// MessagePublisher abstracts queue publishing details.
type MessagePublisher interface {
	Publish(ctx context.Context, reading telemetry.Reading) error
}

// QueueHealth surfaces runtime queue pressure metrics.
type QueueHealth struct {
	Depth       int
	Capacity    int
	Utilization float64
}

// QueueMonitor exposes queue pressure to streamers for adaptive throttling.
type QueueMonitor interface {
	Health(ctx context.Context) QueueHealth
}
