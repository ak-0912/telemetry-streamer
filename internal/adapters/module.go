package adapters

import (
	"go.uber.org/fx"
	csvadapter "telemetry-streamer/internal/adapters/outbound/csv"
	queueadapter "telemetry-streamer/internal/adapters/outbound/queue"
	"telemetry-streamer/internal/infrastructure/config"
	"telemetry-streamer/internal/ports/outbound"
)

func provideReader(cfg config.Config) (outbound.TelemetryReader, error) {
	return csvadapter.NewReader(cfg.CSVPath)
}

func provideQueuePublisher(cfg config.Config) (*queueadapter.Publisher, error) {
	return queueadapter.NewPublisher(cfg.QueueServiceURL)
}

func provideMessagePublisher(p *queueadapter.Publisher) outbound.MessagePublisher {
	return p
}

func provideQueueMonitor(p *queueadapter.Publisher) outbound.QueueMonitor {
	return p
}

var Module = fx.Options(
	fx.Provide(
		provideReader,
		provideQueuePublisher,
		provideMessagePublisher,
		provideQueueMonitor,
	),
)
