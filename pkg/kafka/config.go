package kafka

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config defines the configuration settings for Kafka producers and consumers.
type Config struct {
	Brokers        []string
	ClientID       string
	ConnectTimeout time.Duration
	MaxRetries     int
	RetryInterval  time.Duration
	DLQSuffix      string
	RetrySuffix    string
}

// DefaultConfig returns default Kafka settings.
func DefaultConfig() Config {
	return Config{
		Brokers:        []string{"localhost:9092"},
		ClientID:       "gocart-service",
		ConnectTimeout: 10 * time.Second,
		MaxRetries:     3,
		RetryInterval:  1 * time.Second,
		DLQSuffix:      ".dlq",
		RetrySuffix:    ".retry",
	}
}

// LoadConfigFromEnv loads configuration from environment variables with optional service prefix.
func LoadConfigFromEnv(servicePrefix string) (Config, error) {
	cfg := DefaultConfig()

	brokersStr := getEnvWithPrefix(servicePrefix, "KAFKA_BROKERS")
	if brokersStr != "" {
		parts := strings.Split(brokersStr, ",")
		var brokers []string
		for _, p := range parts {
			trimmed := strings.TrimSpace(p)
			if trimmed != "" {
				brokers = append(brokers, trimmed)
			}
		}
		if len(brokers) > 0 {
			cfg.Brokers = brokers
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_CLIENT_ID"); val != "" {
		cfg.ClientID = val
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_CONNECT_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.ConnectTimeout = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_MAX_RETRIES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			cfg.MaxRetries = n
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_RETRY_INTERVAL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.RetryInterval = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_DLQ_SUFFIX"); val != "" {
		cfg.DLQSuffix = val
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_RETRY_SUFFIX"); val != "" {
		cfg.RetrySuffix = val
	}

	if len(cfg.Brokers) == 0 {
		return cfg, fmt.Errorf("kafka brokers list cannot be empty")
	}

	return cfg, nil
}

func getEnvWithPrefix(prefix, key string) string {
	if prefix != "" {
		if val := os.Getenv(fmt.Sprintf("%s_%s", strings.ToUpper(prefix), key)); val != "" {
			return val
		}
	}
	return os.Getenv(key)
}
