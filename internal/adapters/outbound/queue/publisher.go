package queue

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"sync"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"

	telemetryv1 "telemetry-streamer/api"
	mqv1 "telemetry-streamer/api/mq/v1"
	"telemetry-streamer/internal/domain/telemetry"
	"telemetry-streamer/internal/ports/outbound"
)

var ErrRejectedByQueue = errors.New("message rejected by queue service")
var ErrInvalidQueueServiceURL = errors.New("invalid queue service url")

// Publisher pushes telemetry to the message queue over gRPC (mq.v1.MessageQueueService.Publish).
// QueueMonitor.Health is synthesized from local publish depth (MQ has no Health RPC).
type Publisher struct {
	conn     *grpc.ClientConn
	mqClient mqv1.MessageQueueServiceClient

	topic       string
	keyStrategy string
	keyStatic   string
	capacity    int
	drainEvery  time.Duration

	mu           sync.Mutex
	virtualDepth int

	closeOnce sync.Once
	stopDrain chan struct{}
	drainWG   sync.WaitGroup
}

// NewPublisher dials the MQ gRPC endpoint and configures Publish topic/key behavior.
func NewPublisher(
	queueServiceURL string,
	mqTopic string,
	mqKeyStrategy string,
	mqKeyStatic string,
	virtualCapacity int,
	drainEvery time.Duration,
) (*Publisher, error) {
	target, creds, err := parseQueueTarget(queueServiceURL)
	if err != nil {
		return nil, err
	}
	if virtualCapacity <= 0 {
		virtualCapacity = 1024
	}
	if drainEvery <= 0 {
		drainEvery = 75 * time.Millisecond
	}

	conn, err := grpc.NewClient(target, grpc.WithTransportCredentials(creds))
	if err != nil {
		return nil, fmt.Errorf("grpc dial %q: %w", target, err)
	}

	p := &Publisher{
		conn:        conn,
		mqClient:    mqv1.NewMessageQueueServiceClient(conn),
		topic:       strings.TrimSpace(mqTopic),
		keyStrategy: strings.ToLower(strings.TrimSpace(mqKeyStrategy)),
		keyStatic:   strings.TrimSpace(mqKeyStatic),
		capacity:    virtualCapacity,
		drainEvery:  drainEvery,
		stopDrain:   make(chan struct{}),
	}
	p.drainWG.Add(1)
	go p.runDrainLoop()
	return p, nil
}

// Close stops the synthetic depth drain loop and closes the gRPC connection.
func (p *Publisher) Close() error {
	var closeErr error
	p.closeOnce.Do(func() {
		close(p.stopDrain)
		p.drainWG.Wait()
		if p.conn != nil {
			closeErr = p.conn.Close()
		}
	})
	return closeErr
}

func (p *Publisher) runDrainLoop() {
	defer p.drainWG.Done()
	t := time.NewTicker(p.drainEvery)
	defer t.Stop()
	for {
		select {
		case <-p.stopDrain:
			return
		case <-t.C:
			p.mu.Lock()
			if p.virtualDepth > 0 {
				p.virtualDepth--
			}
			p.mu.Unlock()
		}
	}
}

func parseQueueTarget(raw string) (string, credentials.TransportCredentials, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil, ErrInvalidQueueServiceURL
	}

	if !strings.Contains(raw, "://") {
		return raw, insecure.NewCredentials(), nil
	}

	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", nil, fmt.Errorf("%w: %v", ErrInvalidQueueServiceURL, err)
	}

	var creds credentials.TransportCredentials
	switch strings.ToLower(u.Scheme) {
	case "http", "grpc":
		creds = insecure.NewCredentials()
	case "https", "grpcs":
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	default:
		return "", nil, fmt.Errorf("%w: unsupported scheme %q (use http, https, grpc, grpcs, or host:port)", ErrInvalidQueueServiceURL, u.Scheme)
	}
	return u.Host, creds, nil
}

func readingToProto(r telemetry.Reading) *telemetryv1.TelemetryMessage {
	return &telemetryv1.TelemetryMessage{
		MetricName:          r.MetricName,
		GpuId:               r.GPUId,
		Device:              r.Device,
		Uuid:                r.UUID,
		ModelName:           r.ModelName,
		HostName:            r.HostName,
		Value:               r.Value,
		LabelsRaw:           r.LabelsRaw,
		ProcessedAtUnixNano: r.Timestamp.UnixNano(),
	}
}

func (p *Publisher) messageKey(r telemetry.Reading) string {
	switch p.keyStrategy {
	case "static":
		if p.keyStatic != "" {
			return p.keyStatic
		}
		return p.topic
	case "gpu_id":
		return r.GPUId
	case "metric_name":
		return r.MetricName
	case "metric_gpu":
		return fmt.Sprintf("%s:%s", r.MetricName, r.GPUId)
	case "uuid":
		return r.UUID
	default:
		return r.GPUId
	}
}

// Publish implements outbound.MessagePublisher — sends one TelemetryMessage as MQ Publish payload bytes.
func (p *Publisher) Publish(ctx context.Context, reading telemetry.Reading) error {
	msg := readingToProto(reading)
	payload, err := proto.Marshal(msg)
	if err != nil {
		return err
	}

	req := &mqv1.PublishRequest{
		Topic:   p.topic,
		Key:     p.messageKey(reading),
		Payload: payload,
	}

	_, err = p.mqClient.Publish(ctx, req)
	if err != nil {
		if st, ok := status.FromError(err); ok {
			switch st.Code() {
			case codes.ResourceExhausted, codes.Unavailable, codes.Aborted, codes.DeadlineExceeded:
				p.signalPressure()
				return ErrRejectedByQueue
			}
		}
		p.signalPressure()
		return err
	}

	p.recordEnqueueSuccess()
	return nil
}

func (p *Publisher) signalPressure() {
	p.mu.Lock()
	p.virtualDepth = p.capacity
	p.mu.Unlock()
}

func (p *Publisher) recordEnqueueSuccess() {
	p.mu.Lock()
	if p.virtualDepth < p.capacity {
		p.virtualDepth++
	}
	p.mu.Unlock()
}

// Health implements outbound.QueueMonitor using a local virtual queue depth (MQ API has no Health RPC).
func (p *Publisher) Health(ctx context.Context) outbound.QueueHealth {
	_ = ctx
	p.mu.Lock()
	defer p.mu.Unlock()
	capacity := p.capacity
	if capacity <= 0 {
		capacity = 1
	}
	depth := p.virtualDepth
	if depth > capacity {
		depth = capacity
	}
	util := float64(depth) / float64(capacity)
	return outbound.QueueHealth{
		Depth:       depth,
		Capacity:    capacity,
		Utilization: util,
	}
}
