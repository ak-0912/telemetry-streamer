// Package usecase contains application-level orchestration (use cases) that
// wire domain logic with outbound ports (reader, publisher, monitor).
package usecase

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/observability"
	"telemetry-streamer/internal/ports/outbound"
)

const (
	// Utilization thresholds for tiered throttling.
	throttleCritical = 0.95
	throttleHigh     = 0.85
	throttleModerate = 0.70
	// maxBackoffStep caps exponential backoff (2^6 = 64x base interval).
	maxBackoffStep = 6
)

// StreamTelemetry is the primary use case: it reads telemetry from a source,
// validates each reading, and publishes it to the message queue with adaptive
// backpressure based on queue utilization.
type StreamTelemetry struct {
	reader    outbound.TelemetryReader
	publisher outbound.MessagePublisher
	monitor   outbound.QueueMonitor

	baseInterval time.Duration
	// workerCount controls how many goroutines independently replay the full
	// dataset in parallel, each calling reader.Read separately. This is
	// intentional for load-generation / throughput scaling; every worker
	// processes the entire shard.
	workerCount int

	mu          sync.Mutex
	workerIDSeq int
	workers     map[int]context.CancelFunc
	backoffStep int
}

// ErrInvalidDependency signals that a required dependency (reader, publisher, or monitor) was nil.
var ErrInvalidDependency = errors.New("invalid stream telemetry dependency")

// NewStreamTelemetry constructs the use case. Returns ErrInvalidDependency if any port is nil.
// workerCount defaults to 1, baseInterval defaults to 50ms if invalid values are provided.
func NewStreamTelemetry(
	reader outbound.TelemetryReader,
	publisher outbound.MessagePublisher,
	monitor outbound.QueueMonitor,
	baseInterval time.Duration,
	workerCount int,
) (*StreamTelemetry, error) {
	if reader == nil {
		return nil, fmt.Errorf("%w: reader is required", ErrInvalidDependency)
	}
	if publisher == nil {
		return nil, fmt.Errorf("%w: publisher is required", ErrInvalidDependency)
	}
	if monitor == nil {
		return nil, fmt.Errorf("%w: monitor is required", ErrInvalidDependency)
	}
	if workerCount < 1 {
		workerCount = 1
	}
	if baseInterval <= 0 {
		baseInterval = 50 * time.Millisecond
	}

	return &StreamTelemetry{
		reader:       reader,
		publisher:    publisher,
		monitor:      monitor,
		baseInterval: baseInterval,
		workerCount:  workerCount,
		workers:      make(map[int]context.CancelFunc),
	}, nil
}

// Stream spawns workerCount goroutines and blocks until ctx is cancelled.
// Each worker reads from the TelemetryReader and publishes with adaptive delay.
func (uc *StreamTelemetry) Stream(ctx context.Context) error {
	for i := 0; i < uc.workerCount; i++ {
		uc.addWorker(ctx)
	}
	<-ctx.Done()
	uc.stopAllWorkers()
	return nil
}

// Handle validates a single reading and publishes it. Returns nil on success.
func (uc *StreamTelemetry) Handle(ctx context.Context, reading telemetry.Reading) error {
	if err := telemetry.ValidateReading(reading); err != nil {
		observability.IncValidationError()
		return err
	}
	started := time.Now()
	err := uc.publisher.Publish(ctx, reading)
	observability.ObservePublishLatencyNanos(time.Since(started).Nanoseconds())
	if err != nil {
		observability.IncPublishError()
		slog.Error("publish failed", "err", err)
		return err
	}
	observability.IncPublished()
	return nil
}

func (uc *StreamTelemetry) runWorker(ctx context.Context, workerID int) {
	readings, errs := uc.reader.Read(ctx)
	for {
		select {
		case <-ctx.Done():
			return
		case err, ok := <-errs:
			if ok && err != nil {
				observability.IncReaderError()
				slog.Error("reader error", "worker", workerID, "err", err)
				return
			}
		case reading, ok := <-readings:
			if !ok {
				return
			}
			if wait := uc.currentDelay(ctx); wait > 0 {
				timer := time.NewTimer(wait)
				select {
				case <-ctx.Done():
					timer.Stop()
					return
				case <-timer.C:
				}
			}
			if err := uc.Handle(ctx, reading); err != nil {
				if errors.Is(err, context.Canceled) {
					return
				}
				uc.applyBackoff()
				continue
			}
			uc.recoverBackoff()
		}
	}
}

func (uc *StreamTelemetry) currentDelay(ctx context.Context) time.Duration {
	health := uc.monitor.Health(ctx)
	observability.ObserveQueueUtilization(health.Utilization)
	uc.mu.Lock()
	backoffStep := uc.backoffStep
	uc.mu.Unlock()

	// Tiered delay: increase inter-publish pause as queue fills up to prevent
	// overwhelming the MQ and forcing hard rejects. Thresholds chosen so that
	// the producer slows gradually rather than hitting the critical mark and
	// getting ResourceExhausted errors in bursts.
	delay := uc.baseInterval
	switch {
	case health.Utilization >= throttleCritical:
		delay = uc.baseInterval * 8
	case health.Utilization >= throttleHigh:
		delay = uc.baseInterval * 4
	case health.Utilization >= throttleModerate:
		delay = uc.baseInterval * 2
	}

	// Exponential backoff on consecutive failures (capped at step 6 = 64x base,
	// yielding a max delay of ~3.2 s with 50 ms base). This prevents a failing
	// publisher from hot-looping while still recovering quickly once MQ is healthy.
	for i := 0; i < backoffStep; i++ {
		delay *= 2
	}
	return delay
}

// applyBackoff increments the exponential backoff step (max maxBackoffStep → 64x multiplier).
func (uc *StreamTelemetry) applyBackoff() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.backoffStep < maxBackoffStep {
		uc.backoffStep++
	}
}

func (uc *StreamTelemetry) recoverBackoff() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.backoffStep > 0 {
		uc.backoffStep--
	}
}

func (uc *StreamTelemetry) addWorker(parent context.Context) {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	uc.workerIDSeq++
	id := uc.workerIDSeq
	workerCtx, cancel := context.WithCancel(parent)
	uc.workers[id] = cancel
	go uc.runWorker(workerCtx, id)
}

func (uc *StreamTelemetry) stopAllWorkers() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	for id, cancel := range uc.workers {
		cancel()
		delete(uc.workers, id)
	}
}

