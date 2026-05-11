package queue

import (
	"context"
	"errors"
	"net"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	mqv1 "telemetry-streamer/api/mq/v1"
	"telemetry-streamer/internal/domain/telemetry"
)

type testMQ struct {
	mqv1.UnimplementedMessageQueueServiceServer
	publish func(context.Context, *mqv1.PublishRequest) (*mqv1.PublishResponse, error)
}

func (s *testMQ) Publish(ctx context.Context, in *mqv1.PublishRequest) (*mqv1.PublishResponse, error) {
	if s.publish != nil {
		return s.publish(ctx, in)
	}
	return &mqv1.PublishResponse{Partition: 0, Offset: 1}, nil
}

func startTestMQGRPCServer(t *testing.T, impl mqv1.MessageQueueServiceServer) string {
	t.Helper()
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	srv := grpc.NewServer()
	mqv1.RegisterMessageQueueServiceServer(srv, impl)
	go func() {
		if err := srv.Serve(lis); err != nil {
			t.Logf("grpc serve: %v", err)
		}
	}()
	t.Cleanup(func() { srv.Stop() })
	return "grpc://" + lis.Addr().String()
}

func newTestPublisher(t *testing.T, url string) *Publisher {
	t.Helper()
	p, err := NewPublisher(PublisherConfig{
		QueueServiceURL: url,
		Topic:           "telemetry",
		KeyStrategy:     "gpu_id",
		Capacity:        10,
		DrainInterval:   50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func TestPublisherPublishAndHealth(t *testing.T) {
	t.Parallel()

	addr := startTestMQGRPCServer(t, &testMQ{
		publish: func(_ context.Context, in *mqv1.PublishRequest) (*mqv1.PublishResponse, error) {
			if in.GetTopic() != "telemetry" {
				t.Fatalf("topic: %q", in.GetTopic())
			}
			if in.GetKey() != "0" {
				t.Fatalf("key: %q", in.GetKey())
			}
			if len(in.GetPayload()) == 0 {
				t.Fatal("expected non-empty payload")
			}
			return &mqv1.PublishResponse{Partition: 0, Offset: 42}, nil
		},
	})

	p := newTestPublisher(t, addr)

	err := p.Publish(context.Background(), telemetry.Reading{
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		GPUID:      "0",
		Device:     "nvidia0",
		UUID:       "uuid-0",
		ModelName:  "H100",
		Hostname:   "host-a",
		Value:      10,
		LabelsRaw:  "gpu=0",
		Timestamp:  time.Now(),
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	h := p.Health(context.Background())
	if h.Capacity != 10 {
		t.Fatalf("capacity: %d", h.Capacity)
	}
	if h.Utilization < 0 || h.Utilization > 1 {
		t.Fatalf("utilization: %v", h.Utilization)
	}
}

func TestPublisherRejectResourceExhausted(t *testing.T) {
	t.Parallel()

	addr := startTestMQGRPCServer(t, &testMQ{
		publish: func(context.Context, *mqv1.PublishRequest) (*mqv1.PublishResponse, error) {
			return nil, status.Error(codes.ResourceExhausted, "overload")
		},
	})

	p := newTestPublisher(t, addr)

	err := p.Publish(context.Background(), telemetry.Reading{
		MetricName: "metric",
		GPUID:      "0",
		Timestamp:  time.Now(),
	})
	if !errors.Is(err, ErrRejectedByQueue) {
		t.Fatalf("expected ErrRejectedByQueue, got: %v", err)
	}
}

func TestNewPublisherInvalidURL(t *testing.T) {
	t.Parallel()

	if _, err := NewPublisher(PublisherConfig{QueueServiceURL: "://bad-url", Topic: "t", KeyStrategy: "gpu_id", Capacity: 10, DrainInterval: time.Millisecond}); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestParseQueueTargetHTTPAndHostPort(t *testing.T) {
	t.Parallel()

	target, creds, err := parseQueueTarget("http://127.0.0.1:9999")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if target != "127.0.0.1:9999" || creds == nil {
		t.Fatalf("unexpected target=%q creds=%v", target, creds)
	}

	target2, creds2, err := parseQueueTarget("127.0.0.1:1234")
	if err != nil {
		t.Fatalf("parse host:port: %v", err)
	}
	if target2 != "127.0.0.1:1234" || creds2 == nil {
		t.Fatalf("unexpected target=%q", target2)
	}
}

func TestNewPublisherHostPortWithoutScheme(t *testing.T) {
	t.Parallel()

	addr := startTestMQGRPCServer(t, &testMQ{
		publish: func(context.Context, *mqv1.PublishRequest) (*mqv1.PublishResponse, error) {
			return &mqv1.PublishResponse{}, nil
		},
	})
	hostPort := strings.TrimPrefix(strings.TrimPrefix(addr, "grpc://"), "http://")

	p, err := NewPublisher(PublisherConfig{QueueServiceURL: hostPort, Topic: "telemetry", KeyStrategy: "gpu_id", Capacity: 10, DrainInterval: time.Millisecond})
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	defer func() { _ = p.Close() }()

	if err := p.Publish(context.Background(), telemetry.Reading{
		MetricName: "m",
		GPUID:      "0",
		Timestamp:  time.Now(),
	}); err != nil {
		t.Fatalf("publish: %v", err)
	}
}

func TestMessageKeyStrategies(t *testing.T) {
	t.Parallel()

	r := telemetry.Reading{
		MetricName: "util",
		GPUID:      "7",
		UUID:       "abc-uuid",
		Timestamp:  time.Now(),
	}

	tests := []struct {
		strategy string
		static   string
		wantKey  string
	}{
		{"gpu_id", "", "7"},
		{"metric_name", "", "util"},
		{"metric_gpu", "", "util:7"},
		{"uuid", "", "abc-uuid"},
		{"static", "fixed-key", "fixed-key"},
		{"static", "", "mytopic"},
	}

	for _, tc := range tests {
		tc := tc
		t.Run(tc.strategy, func(t *testing.T) {
			t.Parallel()
			p := &Publisher{topic: "mytopic", keyStrategy: tc.strategy, keyStatic: tc.static}
			if got := p.messageKey(r); got != tc.wantKey {
				t.Fatalf("key got=%q want=%q", got, tc.wantKey)
			}
		})
	}
}
