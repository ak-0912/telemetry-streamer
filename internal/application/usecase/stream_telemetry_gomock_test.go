package usecase

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/ports/outbound"
)

//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../ports/outbound/telemetry_reader.go -destination=./mock_reader_outbound_test.go -package=usecase
//go:generate go run github.com/golang/mock/mockgen@v1.6.0 -source=../../ports/outbound/message_publisher.go -destination=./mock_message_outbound_test.go -package=usecase

func TestRunWorkerPublishSuccessRecoversBackoff(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader := NewMockTelemetryReader(ctrl)
	publisher := NewMockMessagePublisher(ctrl)
	monitor := NewMockQueueMonitor(ctrl)

	readings := make(chan telemetry.Reading, 1)
	errs := make(chan error)
	readings <- telemetry.Reading{MetricName: "gpu.util", GPUID: "0"}
	close(readings)
	close(errs)

	reader.EXPECT().Read(gomock.Any()).Return(readings, errs).Times(1)
	monitor.EXPECT().Health(gomock.Any()).Return(outbound.QueueHealth{Utilization: 0.2}).Times(1)
	publisher.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(nil).Times(1)

	uc, err := NewStreamTelemetry(reader, publisher, monitor, time.Nanosecond, 1)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}
	uc.backoffStep = 1

	uc.runWorker(context.Background(), 1)

	if uc.backoffStep != 0 {
		t.Fatalf("expected backoff to recover, got=%d", uc.backoffStep)
	}
}

func TestRunWorkerPublishFailureAppliesBackoff(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader := NewMockTelemetryReader(ctrl)
	publisher := NewMockMessagePublisher(ctrl)
	monitor := NewMockQueueMonitor(ctrl)

	readings := make(chan telemetry.Reading, 1)
	errs := make(chan error)
	readings <- telemetry.Reading{MetricName: "gpu.util", GPUID: "0"}
	close(readings)
	close(errs)

	reader.EXPECT().Read(gomock.Any()).Return(readings, errs).Times(1)
	monitor.EXPECT().Health(gomock.Any()).Return(outbound.QueueHealth{Utilization: 0.2}).Times(1)
	publisher.EXPECT().Publish(gomock.Any(), gomock.Any()).Return(errors.New("publish failed")).Times(1)

	uc, err := NewStreamTelemetry(reader, publisher, monitor, time.Nanosecond, 1)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	uc.runWorker(context.Background(), 1)

	if uc.backoffStep != 1 {
		t.Fatalf("expected backoff to increase, got=%d", uc.backoffStep)
	}
}

func TestRunWorkerReaderErrorExits(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader := NewMockTelemetryReader(ctrl)
	publisher := NewMockMessagePublisher(ctrl)
	monitor := NewMockQueueMonitor(ctrl)

	readings := make(chan telemetry.Reading)
	errs := make(chan error, 1)
	errs <- errors.New("reader failed")
	close(errs)

	reader.EXPECT().Read(gomock.Any()).Return(readings, errs).Times(1)
	publisher.EXPECT().Publish(gomock.Any(), gomock.Any()).Times(0)

	uc, err := NewStreamTelemetry(reader, publisher, monitor, time.Nanosecond, 1)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	uc.runWorker(context.Background(), 1)
}

func TestStreamStartsWorkersAndStopsOnCancel(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	defer ctrl.Finish()

	reader := NewMockTelemetryReader(ctrl)
	publisher := NewMockMessagePublisher(ctrl)
	monitor := NewMockQueueMonitor(ctrl)

	const workers = 2
	var started sync.WaitGroup
	started.Add(workers)

	reader.EXPECT().Read(gomock.Any()).DoAndReturn(func(ctx context.Context) (<-chan telemetry.Reading, <-chan error) {
		started.Done()
		readings := make(chan telemetry.Reading)
		errs := make(chan error)
		go func() {
			<-ctx.Done()
			close(readings)
			close(errs)
		}()
		return readings, errs
	}).Times(workers)

	uc, err := NewStreamTelemetry(reader, publisher, monitor, time.Nanosecond, workers)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		_ = uc.Stream(ctx)
		close(done)
	}()

	waited := make(chan struct{})
	go func() {
		started.Wait()
		close(waited)
	}()

	select {
	case <-waited:
	case <-time.After(2 * time.Second):
		t.Fatal("workers did not start in time")
	}

	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("stream did not stop in time")
	}
}

