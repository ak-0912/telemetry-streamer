package main

import (
	"go.uber.org/fx"
	"telemetry-streamer/internal/adapters"
	"telemetry-streamer/internal/application"
	"telemetry-streamer/internal/infrastructure"
)

func main() {
	fx.New(
		infrastructure.Module,
		adapters.Module,
		application.Module,
	).Run()
}
