// Package infrastructure wires config loading, probe server, and other
// cross-cutting concerns into the Fx dependency graph.
package infrastructure

import (
	"go.uber.org/fx"
	"telemetry-streamer/internal/infrastructure/config"
)

// Module provides validated Config and starts the HTTP probe/metrics server.
var Module = fx.Options(
	fx.Provide(config.LoadValidated),
	fx.Invoke(runProbeServer),
)
