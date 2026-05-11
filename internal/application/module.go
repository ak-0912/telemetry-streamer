// Package application assembles use-case providers and lifecycle hooks for
// the Fx application container.
package application

import (
	"context"
	"log/slog"
	"sync"

	"go.uber.org/fx"
	"telemetry-streamer/internal/application/usecase"
	"telemetry-streamer/internal/infrastructure/config"
	"telemetry-streamer/internal/ports/outbound"
)

func provideStreamTelemetryUseCase(
	cfg config.Config,
	reader outbound.TelemetryReader,
	publisher outbound.MessagePublisher,
	monitor outbound.QueueMonitor,
) (*usecase.StreamTelemetry, error) {
	return usecase.NewStreamTelemetry(
		reader,
		publisher,
		monitor,
		cfg.StreamInterval,
		cfg.StreamWorkers,
	)
}

func runStreamer(lc fx.Lifecycle, shutdowner fx.Shutdowner, uc *usecase.StreamTelemetry) {
	runCtx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	lc.Append(fx.Hook{
		OnStart: func(context.Context) error {
			wg.Add(1)
			go func() {
				defer wg.Done()
				if err := uc.Stream(runCtx); err != nil {
					slog.Error("streamer stopped with error", "err", err)
					_ = shutdowner.Shutdown()
				}
			}()
			return nil
		},
		OnStop: func(context.Context) error {
			cancel()
			wg.Wait()
			return nil
		},
	})
}

// Module provides the StreamTelemetry use case and starts the streaming lifecycle hook.
var Module = fx.Options(
	fx.Provide(provideStreamTelemetryUseCase),
	fx.Invoke(runStreamer),
)
