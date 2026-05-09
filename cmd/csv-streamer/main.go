// Command csv-streamer is the same streaming app as telemetry-streamer, with convenient
// flags that map to QUEUE_BACKEND / MQ_GRPC_ADDR / CSV_PATH / MQ_TOPIC before Fx starts.
package main

import (
	"flag"
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
		_ = os.Setenv("QUEUE_BACKEND", "grpc")
		_ = os.Setenv("MQ_GRPC_ADDR", strings.TrimSpace(*addr))
	}
	if strings.TrimSpace(*csvPath) != "" {
		_ = os.Setenv("CSV_PATH", strings.TrimSpace(*csvPath))
	}
	if strings.TrimSpace(*topic) != "" {
		_ = os.Setenv("MQ_TOPIC", strings.TrimSpace(*topic))
	}

	fx.New(
		infrastructure.Module,
		adapters.Module,
		application.Module,
	).Run()
}
