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
	GRPC     GRPCConfig
	Database DatabaseConfig
	Return   ReturnConfig
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

type GRPCConfig struct {
	Port               string
	OrderServiceAddr   string
	ProductServiceAddr string
	TimeoutSeconds     time.Duration
}

type ReturnConfig struct {
	ReturnWindowDays int
}

type KafkaConfig struct {
	Brokers  []string
	ClientID string
	Enabled  bool
}

type OutboxConfig struct {
	Enabled      bool
	PollInterval time.Duration
	BatchSize    int
	MaxRetries   int
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

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/return-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "return-service"),
			Environment: GetEnv("ENV", GetEnv("APP_ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "9150"),
		},
		GRPC: GRPCConfig{
			Port:               GetEnv("GRPC_PORT", "50062"),
			OrderServiceAddr:   GetEnv("ORDER_SERVICE_GRPC_ADDR", GetEnv("ORDER_SERVICE_ADDR", "localhost:50059")),
			ProductServiceAddr: GetEnv("PRODUCT_SERVICE_GRPC_ADDR", GetEnv("PRODUCT_SERVICE_ADDR", "localhost:50053")),
			TimeoutSeconds:     time.Duration(GetEnvAsInt("GRPC_TIMEOUT_SECONDS", 5)) * time.Second,
		},
		Return: ReturnConfig{
			ReturnWindowDays: GetEnvAsInt("RETURN_WINDOW_DAYS", 7),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("RETURN_SERVICE_DATABASE_URL", GetEnv("RETURNS_SERVICE_DATABASE_URL", GetEnv("DATABASE_URL", "postgresql://postgres:password@localhost:5432/return-service"))),
			MaxConns:       int32(GetEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:       int32(GetEnvAsInt("DB_MIN_CONNS", 2)),
			AutoMigrate:    GetEnvAsBool("DB_AUTO_MIGRATE", true),
			MigrationsPath: GetEnv("DB_MIGRATIONS_PATH", "./migrations"),
		},
		Kafka: KafkaConfig{
			Brokers:  GetEnvAsStringSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			ClientID: GetEnv("KAFKA_CLIENT_ID", "return-service"),
			Enabled:  GetEnvAsBool("KAFKA_ENABLED", true),
		},
		Outbox: OutboxConfig{
			Enabled:      GetEnvAsBool("OUTBOX_ENABLED", true),
			PollInterval: GetEnvAsDuration("OUTBOX_POLL_INTERVAL", 2*time.Second),
			BatchSize:    GetEnvAsInt("OUTBOX_BATCH_SIZE", 50),
			MaxRetries:   GetEnvAsInt("OUTBOX_MAX_RETRIES", 5),
		},
		Logger: LoggerConfig{
			Level:  GetEnv("LOG_LEVEL", "debug"),
			Format: GetEnv("LOG_FORMAT", "json"),
		},
		Tracing: TracingConfig{
			Enabled:      GetEnvAsBool("TRACING_ENABLED", false),
			Exporter:     GetEnv("TRACING_EXPORTER", "stdout"),
			OTLPEndpoint: GetEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
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
	val, err := time.ParseDuration(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

func GetEnvAsStringSlice(key string, defaultValue []string) []string {
	valStr := os.Getenv(key)
	if valStr == "" {
		return defaultValue
	}
	parts := strings.Split(valStr, ",")
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
