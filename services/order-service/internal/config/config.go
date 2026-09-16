package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	Database DatabaseConfig
	Logger   LoggerConfig
	Tracing  TracingConfig
	Kafka    KafkaConfig
	Outbox   OutboxConfig
}

type AppConfig struct {
	Name        string
	Environment string
	Version     string
}

type HTTPConfig struct {
	Port string
}

type DatabaseConfig struct {
	URL            string
	MaxConns       int32
	MinConns       int32
	AutoMigrate    bool
	MigrationsPath string
}

type LoggerConfig struct {
	Level  string
	Format string
}

type TracingConfig struct {
	Enabled      bool
	Exporter     string
	OTLPEndpoint string
}

type KafkaConfig struct {
	Brokers  []string
	ClientID string
}

type OutboxConfig struct {
	PollInterval time.Duration
	BatchSize    int
	MaxRetries   int
}

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/order-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "order-service"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "8500"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("DATABASE_URL", ""),
			MaxConns:       int32(GetEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:       int32(GetEnvAsInt("DB_MIN_CONNS", 2)),
			AutoMigrate:    GetEnvAsBool("DB_AUTO_MIGRATE", true),
			MigrationsPath: GetEnv("DB_MIGRATIONS_PATH", "./migrations"),
		},
		Logger: LoggerConfig{
			Level:  GetEnv("LOG_LEVEL", "debug"),
			Format: GetEnv("LOG_FORMAT", "json"),
		},
		Tracing: TracingConfig{
			Enabled:      GetEnvAsBool("TRACING_ENABLED", true),
			Exporter:     GetEnv("TRACING_EXPORTER", "stdout"),
			OTLPEndpoint: GetEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		},
		Kafka: KafkaConfig{
			Brokers:  GetEnvAsStringSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			ClientID: GetEnv("KAFKA_CLIENT_ID", "order-service"),
		},
		Outbox: OutboxConfig{
			PollInterval: GetEnvAsDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
			BatchSize:    GetEnvAsInt("OUTBOX_BATCH_SIZE", 50),
			MaxRetries:   GetEnvAsInt("OUTBOX_MAX_RETRIES", 5),
		},
	}
}

func GetEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}

func GetEnvAsInt(key string, defaultValue int) int {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

func GetEnvAsBool(key string, defaultValue bool) bool {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.ParseBool(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

func GetEnvAsDuration(key string, defaultValue time.Duration) time.Duration {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(valStr)
	if err != nil {
		return defaultValue
	}
	return d
}

func GetEnvAsStringSlice(key string, defaultValue []string) []string {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	parts := strings.Split(valStr, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			result = append(result, t)
		}
	}
	if len(result) == 0 {
		return defaultValue
	}
	return result
}
