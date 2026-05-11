// Package adapters wires concrete adapter implementations (CSV reader, gRPC
// queue publisher) into the Fx dependency graph as outbound port providers.
package adapters

import (
	"context"

	"go.uber.org/fx"
	csvadapter "telemetry-streamer/internal/adapters/outbound/csv"
	queueadapter "telemetry-streamer/internal/adapters/outbound/queue"
	"telemetry-streamer/internal/infrastructure/config"
	"telemetry-streamer/internal/ports/outbound"
)

func provideReader(cfg config.Config) (outbound.TelemetryReader, error) {
	return csvadapter.NewReaderWithShard(cfg.CSVPath, cfg.ShardTotal, cfg.ShardIndex)
}

func provideQueuePublisher(lc fx.Lifecycle, cfg config.Config) (*queueadapter.Publisher, error) {
	p, err := queueadapter.NewPublisher(queueadapter.PublisherConfig{
		QueueServiceURL: cfg.QueueServiceURL,
		Topic:           cfg.MQTopic,
		KeyStrategy:     cfg.MQKeyStrategy,
		KeyStatic:       cfg.MQKeyStatic,
		Capacity:        cfg.QueueCapacity,
		DrainInterval:   cfg.QueueConsumeDelay,
	})
	if err != nil {
		return nil, err
	}
	lc.Append(fx.Hook{
		OnStop: func(context.Context) error { return p.Close() },
	})
	return p, nil
}

func provideMessagePublisher(p *queueadapter.Publisher) outbound.MessagePublisher {
	return p
}

func provideQueueMonitor(p *queueadapter.Publisher) outbound.QueueMonitor {
	return p
}

// Module provides TelemetryReader, MessagePublisher, and QueueMonitor to the Fx container.
var Module = fx.Options(
	fx.Provide(
		provideReader,
		provideQueuePublisher,
		provideMessagePublisher,
		provideQueueMonitor,
	),
)
