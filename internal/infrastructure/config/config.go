package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config collects runtime settings for infrastructure wiring.
type Config struct {
	CSVPath            string
	StreamInterval     time.Duration
	ProbeAddr          string
	QueueServiceURL    string
	QueueCapacity      int
	QueueHighWatermark float64
	QueueCriticalMark  float64
	QueueConsumeDelay  time.Duration
	StreamWorkers      int
	// MQ gRPC publish settings (see api/mq/v1/message_queue.proto).
	MQTopic       string
	MQKeyStrategy string
	MQKeyStatic   string
}

var ErrInvalidConfig = errors.New("invalid configuration")

func Load() Config {
	queueServiceURL := getString("QUEUE_SERVICE_URL", "http://127.0.0.1:8081")
	if shouldDialMQViaGRPCAddr() {
		if u := mqGRPCDialURL(getString("MQ_GRPC_ADDR", "")); u != "" {
			queueServiceURL = u
		}
	}

	return Config{
		CSVPath:            getString("CSV_PATH", "dcgm_metrics_20250718_134233.csv"),
		StreamInterval:     getDurationMS("STREAM_INTERVAL_MS", 50),
		ProbeAddr:          getString("PROBE_ADDR", ":8080"),
		QueueServiceURL:    queueServiceURL,
		QueueCapacity:      getInt("QUEUE_CAPACITY", 1024),
		QueueHighWatermark: getFloat("QUEUE_HIGH_WATERMARK", 0.80),
		QueueCriticalMark:  getFloat("QUEUE_CRITICAL_WATERMARK", 0.95),
		QueueConsumeDelay:  getDurationMS("QUEUE_CONSUME_DELAY_MS", 75),
		StreamWorkers:      getInt("STREAM_WORKERS", 1),
		MQTopic:            getString("MQ_TOPIC", "telemetry"),
		MQKeyStrategy:      getString("MQ_KEY_STRATEGY", "gpu_id"),
		MQKeyStatic:        getString("MQ_KEY_STATIC", ""),
	}
}

func LoadValidated() (Config, error) {
	cfg := Load()
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func (c Config) Validate() error {
	if c.CSVPath == "" {
		return fmt.Errorf("%w: CSV_PATH is required", ErrInvalidConfig)
	}
	if c.StreamInterval <= 0 {
		return fmt.Errorf("%w: STREAM_INTERVAL_MS must be > 0", ErrInvalidConfig)
	}
	if c.ProbeAddr == "" {
		return fmt.Errorf("%w: PROBE_ADDR is required", ErrInvalidConfig)
	}
	if c.StreamWorkers <= 0 {
		return fmt.Errorf("%w: STREAM_WORKERS must be > 0", ErrInvalidConfig)
	}
	if c.QueueHighWatermark <= 0 || c.QueueHighWatermark >= 1 {
		return fmt.Errorf("%w: QUEUE_HIGH_WATERMARK must be in (0,1)", ErrInvalidConfig)
	}
	if c.QueueCriticalMark <= 0 || c.QueueCriticalMark >= 1 {
		return fmt.Errorf("%w: QUEUE_CRITICAL_WATERMARK must be in (0,1)", ErrInvalidConfig)
	}
	if c.QueueCriticalMark <= c.QueueHighWatermark {
		return fmt.Errorf("%w: QUEUE_CRITICAL_WATERMARK must be > QUEUE_HIGH_WATERMARK", ErrInvalidConfig)
	}
	parsedURL, err := url.ParseRequestURI(c.QueueServiceURL)
	if err != nil {
		return fmt.Errorf("%w: QUEUE_SERVICE_URL is invalid: %v", ErrInvalidConfig, err)
	}
	switch parsedURL.Scheme {
	case "http", "https", "grpc", "grpcs":
	default:
		return fmt.Errorf("%w: QUEUE_SERVICE_URL must use http, https, grpc, or grpcs", ErrInvalidConfig)
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("%w: QUEUE_SERVICE_URL host is required", ErrInvalidConfig)
	}
	if strings.TrimSpace(c.MQTopic) == "" {
		return fmt.Errorf("%w: MQ_TOPIC is required", ErrInvalidConfig)
	}
	switch strings.ToLower(strings.TrimSpace(c.MQKeyStrategy)) {
	case "static", "gpu_id", "metric_name", "metric_gpu", "uuid":
	default:
		return fmt.Errorf("%w: MQ_KEY_STRATEGY must be one of: static, gpu_id, metric_name, metric_gpu, uuid", ErrInvalidConfig)
	}
	return nil
}

// shouldDialMQViaGRPCAddr is true when QUEUE_BACKEND=grpc and MQ_GRPC_ADDR is set; then
// QueueServiceURL is derived from MQ_GRPC_ADDR (overriding QUEUE_SERVICE_URL for that dial target).
func shouldDialMQViaGRPCAddr() bool {
	if !strings.EqualFold(strings.TrimSpace(os.Getenv("QUEUE_BACKEND")), "grpc") {
		return false
	}
	return strings.TrimSpace(os.Getenv("MQ_GRPC_ADDR")) != ""
}

func mqGRPCDialURL(addr string) string {
	addr = strings.TrimSpace(addr)
	if addr == "" {
		return ""
	}
	if strings.Contains(addr, "://") {
		if u, err := url.Parse(addr); err == nil && u.Scheme != "" && u.Host != "" {
			return addr
		}
	}
	return "grpc://" + addr
}

func getString(key, defaultValue string) string {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	return value
}

func getInt(key string, defaultValue int) int {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(value)
	if err != nil {
		return defaultValue
	}
	return n
}

func getFloat(key string, defaultValue float64) float64 {
	value := os.Getenv(key)
	if value == "" {
		return defaultValue
	}
	f, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return defaultValue
	}
	return f
}

func getDurationMS(key string, defaultMS int) time.Duration {
	value := os.Getenv(key)
	if value == "" {
		return time.Duration(defaultMS) * time.Millisecond
	}
	ms, err := strconv.Atoi(value)
	if err != nil {
		return time.Duration(defaultMS) * time.Millisecond
	}
	return time.Duration(ms) * time.Millisecond
}
