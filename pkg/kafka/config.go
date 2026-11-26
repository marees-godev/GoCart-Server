package kafka

import (
	"crypto/tls"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// Config defines the configuration settings for Kafka producers and consumers.
type Config struct {
	Brokers            []string
	ClientID           string
	ConnectTimeout     time.Duration
	MaxRetries         int
	RetryInterval      time.Duration
	DLQSuffix          string
	RetrySuffix        string
	DLQTopic           string
	User               string
	Password           string
	SASLMechanism      string
	TLSEnabled         bool
	InsecureSkipVerify bool
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
		DLQTopic:       "gocart.dlq",
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

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_USER"); val != "" {
		cfg.User = val
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_PASSWORD"); val != "" {
		cfg.Password = val
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_SASL_MECHANISM"); val != "" {
		cfg.SASLMechanism = val
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_TLS"); val != "" {
		cfg.TLSEnabled = val == "true" || val == "1"
	}

	if val := getEnvWithPrefix(servicePrefix, "KAFKA_TLS_INSECURE_SKIP_VERIFY"); val != "" {
		cfg.InsecureSkipVerify = val == "true" || val == "1"
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

func buildSASLMechanism(mechType, username, password string) (sasl.Mechanism, error) {
	switch strings.ToUpper(strings.TrimSpace(mechType)) {
	case "PLAIN":
		return plain.Mechanism{
			Username: username,
			Password: password,
		}, nil
	case "SCRAM-SHA-256":
		return scram.Mechanism(scram.SHA256, username, password)
	case "SCRAM-SHA-512", "":
		return scram.Mechanism(scram.SHA512, username, password)
	default:
		return scram.Mechanism(scram.SHA512, username, password)
	}
}

func buildTLSConfig(insecure bool) *tls.Config {
	return &tls.Config{
		MinVersion:         tls.VersionTLS12,
		InsecureSkipVerify: insecure,
	}
}

func resolveAuthDefaults(cfg *Config) {
	if cfg.User == "" {
		cfg.User = os.Getenv("KAFKA_USER")
	}
	if cfg.Password == "" {
		cfg.Password = os.Getenv("KAFKA_PASSWORD")
	}
	if cfg.SASLMechanism == "" {
		cfg.SASLMechanism = os.Getenv("KAFKA_SASL_MECHANISM")
	}
	if !cfg.TLSEnabled {
		val := os.Getenv("KAFKA_TLS")
		if val == "true" || val == "1" || (cfg.User != "" && cfg.Password != "") {
			cfg.TLSEnabled = true
		}
	}
	if !cfg.InsecureSkipVerify {
		val := os.Getenv("KAFKA_TLS_INSECURE_SKIP_VERIFY")
		if val == "true" || val == "1" || (cfg.User != "" && cfg.Password != "") {
			cfg.InsecureSkipVerify = true
		}
	}
}
