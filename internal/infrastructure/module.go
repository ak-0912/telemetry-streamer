package infrastructure

import (
	"go.uber.org/fx"
	"telemetry-streamer/internal/infrastructure/config"
)

var Module = fx.Options(
	fx.Provide(config.LoadValidated),
	fx.Invoke(runProbeServer),
)
