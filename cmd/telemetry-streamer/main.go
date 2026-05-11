// Binary telemetry-streamer reads GPU telemetry from a CSV source and publishes
// each record to a message queue over gRPC with adaptive backpressure.
package main

import (
	"flag"
	"log/slog"
	"os"
	"strings"

	"go.uber.org/fx"
	"telemetry-streamer/internal/adapters"
	"telemetry-streamer/internal/application"
	"telemetry-streamer/internal/infrastructure"
)

func main() {
	addr := flag.String("addr", "", "MQ gRPC host:port (e.g. telemetry-message-queue:50051 or host.docker.internal:50051); sets QUEUE_BACKEND=grpc and MQ_GRPC_ADDR")
	csvPath := flag.String("csv", "", "CSV file path; sets CSV_PATH")
	topic := flag.String("topic", "", "MQ topic; sets MQ_TOPIC")
	flag.Parse()

	if strings.TrimSpace(*addr) != "" {
		setEnv("QUEUE_BACKEND", "grpc")
		setEnv("MQ_GRPC_ADDR", strings.TrimSpace(*addr))
	}
	if strings.TrimSpace(*csvPath) != "" {
		setEnv("CSV_PATH", strings.TrimSpace(*csvPath))
	}
	if strings.TrimSpace(*topic) != "" {
		setEnv("MQ_TOPIC", strings.TrimSpace(*topic))
	}

	fx.New(
		infrastructure.Module,
		adapters.Module,
		application.Module,
	).Run()
}

func setEnv(key, value string) {
	if err := os.Setenv(key, value); err != nil {
		slog.Error("failed to set env", "key", key, "err", err)
	}
}
