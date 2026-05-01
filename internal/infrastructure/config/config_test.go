package config

import "testing"

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
	}
	if err := cfg.Validate(); err != nil {
		t.Fatalf("expected valid config, got: %v", err)
	}
}

func TestConfigValidateInvalid(t *testing.T) {
	t.Parallel()

	cases := []Config{
		{CSVPath: "", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "bad://", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 0, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95},
		{CSVPath: "a.csv", ProbeAddr: ":8080", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.95, QueueCriticalMark: 0.9},
		{CSVPath: "a.csv", ProbeAddr: "", QueueServiceURL: "http://127.0.0.1:8081", StreamInterval: 1, StreamWorkers: 1, QueueHighWatermark: 0.8, QueueCriticalMark: 0.95},
	}
	for _, tc := range cases {
		if err := tc.Validate(); err == nil {
			t.Fatalf("expected config validation error for %+v", tc)
		}
	}
}
