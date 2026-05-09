package config

import (
	"testing"
)

func TestConfigValidate(t *testing.T) {
	t.Parallel()

	cfg := Config{
		CSVPath:            "data.csv",
		StreamInterval:     1,
		ProbeAddr:          ":8080",
		QueueServiceURL:    "http://127.0.0.1:8081",
		QueueHighWatermark: 0.8,
		QueueCriticalMark:  0.95,
		StreamWorkers:      1,
		MQTopic:            "telemetry",
		MQKeyStrategy:      "gpu_id",
		MQKeyStatic:        "",
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}

	cfgGRPC := cfg
	cfgGRPC.QueueServiceURL = "grpc://queue.default.svc.cluster.local:8081"
	if err := cfgGRPC.Validate(); err != nil {
		t.Fatalf("expected valid grpc URL config, got: %v", err)
	}
}

func TestLoadUsesMQGRPCAddrWhenQueueBackendGRPC(t *testing.T) {
	t.Setenv("QUEUE_BACKEND", "grpc")
	t.Setenv("MQ_GRPC_ADDR", "telemetry-message-queue:50051")
	t.Setenv("QUEUE_SERVICE_URL", "http://should-be-overridden:1")

	cfg := Load()
	if want := "grpc://telemetry-message-queue:50051"; cfg.QueueServiceURL != want {
		t.Fatalf("QueueServiceURL got=%q want=%q", cfg.QueueServiceURL, want)
	}
}

func TestLoadMQGRPCAddrWithScheme(t *testing.T) {
	t.Setenv("QUEUE_BACKEND", "grpc")
	t.Setenv("MQ_GRPC_ADDR", "grpcs://mq.example.com:443")

	cfg := Load()
	if want := "grpcs://mq.example.com:443"; cfg.QueueServiceURL != want {
		t.Fatalf("QueueServiceURL got=%q want=%q", cfg.QueueServiceURL, want)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	t.Parallel()

	cases := []Config{
		{CSVPath: "", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "bad://", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 0, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.95, QueueCriticalMark: 0.9, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: "", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "ftp://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "", MQKeyStrategy: "gpu_id"},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95, MQTopic: "t", MQKeyStrategy: "invalid"},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected config validation error for %+v", tc)
		}
	}
}
