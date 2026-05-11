package usecase

import (
	"context"
	"testing"
	"time"

	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/ports/outbound"
)

type fakeReader struct{}

func (fakeReader) Read(context.Context) (<-chan telemetry.Reading, <-chan error) {
	readings := make(chan telemetry.Reading)
	errs := make(chan error)
	close(readings)
	close(errs)
	return readings, errs
}

type fakePublisher struct{ called bool }

func (f *fakePublisher) Publish(context.Context, telemetry.Reading) error {
	f.called = true
	return nil
}

type fakeMonitor struct{ health outbound.QueueHealth }

func (m fakeMonitor) Health(context.Context) outbound.QueueHealth { return m.health }

func TestCurrentDelayAppliesUtilizationAndBackoff(t *testing.T) {
	t.Parallel()

	pub := &fakePublisher{}
	uc, err := NewStreamTelemetry(
		fakeReader{},
		pub,
		fakeMonitor{health: outbound.QueueHealth{Utilization: 0.9}},
		10*time.Millisecond,
		1,
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	uc.backoffStep = 2 // *2 *2
	got := uc.currentDelay(context.Background())
	want := 160 * time.Millisecond // base(10) * 4(util) * 4(backoff)
	if got != want {
		t.Fatalf("unexpected delay got=%v want=%v", got, want)
	}
}

func TestHandleValidatesBeforePublish(t *testing.T) {
	t.Parallel()

	pub := &fakePublisher{}
	uc, err := NewStreamTelemetry(
		fakeReader{},
		pub,
		fakeMonitor{},
		10*time.Millisecond,
		1,
	)
	if err != nil {
		t.Fatalf("unexpected constructor error: %v", err)
	}

	err = uc.Handle(context.Background(), telemetry.Reading{
		MetricName: "metric",
		GPUID:      "0",
	})
	if err != nil {
		t.Fatalf("unexpected handle error: %v", err)
	}
	if !pub.called {
		t.Fatal("expected publisher to be called")
	}

	pub.called = false
	err = uc.Handle(context.Background(), telemetry.Reading{
		MetricName: "",
		GPUID:      "0",
	})
	if err == nil {
		t.Fatal("expected validation error")
	}
	if pub.called {
		t.Fatal("publisher should not be called when validation fails")
	}
}

func TestNewStreamTelemetryRejectsNilDependencies(t *testing.T) {
	t.Parallel()

	if _, err := NewStreamTelemetry(nil, &fakePublisher{}, fakeMonitor{}, time.Millisecond, 1); err == nil {
		t.Fatal("expected error for nil reader")
	}
	if _, err := NewStreamTelemetry(fakeReader{}, nil, fakeMonitor{}, time.Millisecond, 1); err == nil {
		t.Fatal("expected error for nil publisher")
	}
	if _, err := NewStreamTelemetry(fakeReader{}, &fakePublisher{}, nil, time.Millisecond, 1); err == nil {
		t.Fatal("expected error for nil monitor")
	}
}
