// Package outbound defines port interfaces that adapters must implement
// for the application layer to read telemetry and publish messages.
package outbound

import (
	"context"

	"telemetry-streamer/internal/domain/telemetry"
)

// MessagePublisher abstracts queue publishing details.
// Implementations must be safe for concurrent use.
type MessagePublisher interface {
	// Publish sends a single telemetry reading to the message queue.
	// Returns nil on success; callers may retry on transient errors.
	Publish(ctx context.Context, reading telemetry.Reading) error
}

// QueueHealth surfaces runtime queue pressure metrics used by adaptive throttling.
type QueueHealth struct {
	// Depth is the current number of messages in flight or buffered.
	Depth int
	// Capacity is the maximum number of messages the queue can hold.
	Capacity int
	// Utilization is Depth/Capacity, always in [0.0, 1.0].
	Utilization float64
}

// QueueMonitor exposes queue pressure to streamers for adaptive throttling.
type QueueMonitor interface {
	// Health returns a snapshot of queue pressure; called before each publish.
	Health(ctx context.Context) QueueHealth
}
