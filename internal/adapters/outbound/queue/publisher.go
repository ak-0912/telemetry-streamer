package queue

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/ports/outbound"
)

var ErrRejectedByQueue = errors.New("message rejected by queue service")
var ErrInvalidQueueServiceURL = errors.New("invalid queue service url")
var ErrMalformedQueueResponse = errors.New("malformed queue response")

// Publisher pushes telemetry to remote custom queue over Connect RPC.
type Publisher struct {
	enqueueClient *connect.Client[structpb.Struct, structpb.Struct]
	healthClient  *connect.Client[emptypb.Empty, structpb.Struct]
}

func NewPublisher(queueServiceURL string) (*Publisher, error) {
	if _, err := url.ParseRequestURI(queueServiceURL); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidQueueServiceURL, err)
	}
	httpClient := &http.Client{}
	return &Publisher{
		enqueueClient: connect.NewClient[structpb.Struct, structpb.Struct](httpClient, fmt.Sprintf("%s%s", queueServiceURL, EnqueueProcedure)),
		healthClient:  connect.NewClient[emptypb.Empty, structpb.Struct](httpClient, fmt.Sprintf("%s%s", queueServiceURL, HealthProcedure)),
	}, nil
}

func (p *Publisher) Publish(ctx context.Context, reading telemetry.Reading) error {
	msg, err := structpb.NewStruct(map[string]any{
		"metric_name":             reading.MetricName,
		"gpu_id":                  reading.GPUId,
		"device":                  reading.Device,
		"uuid":                    reading.UUID,
		"model_name":              reading.ModelName,
		"host_name":               reading.HostName,
		"value":                   reading.Value,
		"labels_raw":              reading.LabelsRaw,
		"processed_at_unix_nano":  reading.Timestamp.UnixNano(),
		"processed_at_unix_milli": reading.Timestamp.UnixMilli(),
	})
	if err != nil {
		return err
	}
	resp, err := p.enqueueClient.CallUnary(ctx, connect.NewRequest(msg))
	if err != nil {
		return err
	}
	acceptedValue, ok := resp.Msg.GetFields()["accepted"]
	if !ok || acceptedValue == nil {
		return fmt.Errorf("%w: missing accepted field", ErrMalformedQueueResponse)
	}
	if !acceptedValue.GetBoolValue() {
		return ErrRejectedByQueue
	}
	return nil
}

func (p *Publisher) Health(ctx context.Context) outbound.QueueHealth {
	resp, err := p.healthClient.CallUnary(ctx, connect.NewRequest(&emptypb.Empty{}))
	if err != nil {
		return outbound.QueueHealth{}
	}
	fields := resp.Msg.GetFields()
	depthField, hasDepth := fields["depth"]
	capacityField, hasCapacity := fields["capacity"]
	utilField, hasUtil := fields["utilization"]
	if !hasDepth || !hasCapacity || !hasUtil || depthField == nil || capacityField == nil || utilField == nil {
		return outbound.QueueHealth{}
	}
	depth := int(depthField.GetNumberValue())
	capacity := int(capacityField.GetNumberValue())
	util := utilField.GetNumberValue()
	if capacity <= 0 || util < 0 || util > 1 {
		return outbound.QueueHealth{}
	}
	return outbound.QueueHealth{
		Depth:       depth,
		Capacity:    capacity,
		Utilization: util,
	}
}
