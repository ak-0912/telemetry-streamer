package queue

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"telemetry-streamer/internal/domain/telemetry"
)

func TestPublisherPublishAndHealth(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle(EnqueueProcedure, connect.NewUnaryHandler(
		EnqueueProcedure,
		func(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
			msg, err := structpb.NewStruct(map[string]any{"accepted": true})
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(msg), nil
		},
	))
	mux.Handle(HealthProcedure, connect.NewUnaryHandler(
		HealthProcedure,
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[structpb.Struct], error) {
			msg, err := structpb.NewStruct(map[string]any{
				"depth":       4,
				"capacity":    10,
				"utilization": 0.4,
			})
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(msg), nil
		},
	))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewPublisher(srv.URL)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	err = p.Publish(context.Background(), telemetry.Reading{
		MetricName: "DCGM_FI_DEV_GPU_UTIL",
		GPUId:      "0",
		Device:     "nvidia0",
		UUID:       "uuid-0",
		ModelName:  "H100",
		HostName:   "host-a",
		Value:      10,
		LabelsRaw:  "gpu=0",
		Timestamp:  time.Now(),
	})
	if err != nil {
		t.Fatalf("publish failed: %v", err)
	}

	health := p.Health(context.Background())
	if health.Depth != 4 || health.Capacity != 10 {
		t.Fatalf("unexpected health values: %+v", health)
	}
	if health.Utilization != 0.4 {
		t.Fatalf("unexpected utilization: %v", health.Utilization)
	}
}

func TestPublisherRejectResponse(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle(EnqueueProcedure, connect.NewUnaryHandler(
		EnqueueProcedure,
		func(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
			msg, err := structpb.NewStruct(map[string]any{"accepted": false})
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(msg), nil
		},
	))
	mux.Handle(HealthProcedure, connect.NewUnaryHandler(
		HealthProcedure,
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[structpb.Struct], error) {
			msg, err := structpb.NewStruct(map[string]any{})
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(msg), nil
		},
	))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewPublisher(srv.URL)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	err = p.Publish(context.Background(), telemetry.Reading{
		MetricName: "metric",
		GPUId:      "0",
		Timestamp:  time.Now(),
	})
	if err == nil {
		t.Fatal("expected queue rejection error")
	}
	if err != ErrRejectedByQueue {
		t.Fatalf("expected ErrRejectedByQueue, got: %v", err)
	}
}

func TestNewPublisherInvalidURL(t *testing.T) {
	t.Parallel()

	if _, err := NewPublisher("://bad-url"); err == nil {
		t.Fatal("expected invalid URL error")
	}
}

func TestPublisherMalformedResponse(t *testing.T) {
	t.Parallel()

	mux := http.NewServeMux()
	mux.Handle(EnqueueProcedure, connect.NewUnaryHandler(
		EnqueueProcedure,
		func(context.Context, *connect.Request[structpb.Struct]) (*connect.Response[structpb.Struct], error) {
			msg, err := structpb.NewStruct(map[string]any{}) // missing accepted
			if err != nil {
				return nil, err
			}
			return connect.NewResponse(msg), nil
		},
	))
	mux.Handle(HealthProcedure, connect.NewUnaryHandler(
		HealthProcedure,
		func(context.Context, *connect.Request[emptypb.Empty]) (*connect.Response[structpb.Struct], error) {
			msg, _ := structpb.NewStruct(map[string]any{})
			return connect.NewResponse(msg), nil
		},
	))
	srv := httptest.NewServer(mux)
	defer srv.Close()

	p, err := NewPublisher(srv.URL)
	if err != nil {
		t.Fatalf("new publisher: %v", err)
	}
	err = p.Publish(context.Background(), telemetry.Reading{
		MetricName: "metric",
		GPUId:      "0",
		Timestamp:  time.Now(),
	})
	if err == nil {
		t.Fatal("expected malformed response error")
	}
}
