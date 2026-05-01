package usecase

import (
	"context"
	"errors"
	"fmt"
	"log"
	"sync"
	"time"

	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/observability"
	"telemetry-streamer/internal/ports/outbound"
)

type StreamTelemetry struct {
	reader    outbound.TelemetryReader
	publisher outbound.MessagePublisher
	monitor   outbound.QueueMonitor

	baseInterval time.Duration
	workerCount  int

	mu          sync.Mutex
	workerIDSeq int
	workers     map[int]context.CancelFunc
	backoffStep int
}

var ErrInvalidDependency = errors.New("invalid stream telemetry dependency")

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

func (uc *StreamTelemetry) Stream(ctx context.Context) error {
	for i := 0; i < uc.workerCount; i++ {
		uc.addWorker(ctx)
	}
	<-ctx.Done()
	uc.stopAllWorkers()
	return nil
}

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
				log.Printf("worker=%d reader error: %v", workerID, err)
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

	delay := uc.baseInterval
	switch {
	case health.Utilization >= 0.95:
		delay = uc.baseInterval * 8
	case health.Utilization >= 0.85:
		delay = uc.baseInterval * 4
	case health.Utilization >= 0.70:
		delay = uc.baseInterval * 2
	}

	for i := 0; i < backoffStep; i++ {
		delay *= 2
	}
	return delay
}

func (uc *StreamTelemetry) applyBackoff() {
	uc.mu.Lock()
	defer uc.mu.Unlock()
	if uc.backoffStep < 6 {
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

