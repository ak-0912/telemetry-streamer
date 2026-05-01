package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strconv"
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
}

var ErrInvalidConfig = errors.New("invalid configuration")

func Load() Config {
	return Config{
		CSVPath:            getString("CSV_PATH", "dcgm_metrics_20250718_134233.csv"),
		StreamInterval:     getDurationMS("STREAM_INTERVAL_MS", 50),
		ProbeAddr:          getString("PROBE_ADDR", ":8080"),
		QueueServiceURL:    getString("QUEUE_SERVICE_URL", "http://127.0.0.1:8081"),
		QueueCapacity:      getInt("QUEUE_CAPACITY", 1024),
		QueueHighWatermark: getFloat("QUEUE_HIGH_WATERMARK", 0.80),
		QueueCriticalMark:  getFloat("QUEUE_CRITICAL_WATERMARK", 0.95),
		QueueConsumeDelay:  getDurationMS("QUEUE_CONSUME_DELAY_MS", 75),
		StreamWorkers:      getInt("STREAM_WORKERS", 1),
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
	if parsedURL.Scheme != "http" && parsedURL.Scheme != "https" {
		return fmt.Errorf("%w: QUEUE_SERVICE_URL must use http or https", ErrInvalidConfig)
	}
	if parsedURL.Host == "" {
		return fmt.Errorf("%w: QUEUE_SERVICE_URL host is required", ErrInvalidConfig)
	}
	return nil
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
