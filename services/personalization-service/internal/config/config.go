package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	GRPC     GRPCConfig
	Database DatabaseConfig
	Kafka    KafkaConfig
	Logger   LoggerConfig
	Tracing  TracingConfig
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

type AppConfig struct {
	Name        string
	Environment string
	Version     string
}

type HTTPConfig struct {
	Port string
}

type GRPCConfig struct {
	Port string
}

type DatabaseConfig struct {
	URL            string
	MaxConns       int32
	MinConns       int32
	AutoMigrate    bool
	MigrationsPath string
}

type KafkaConfig struct {
	Brokers  []string
	ClientID string
	GroupID  string
	Enabled  bool
}

func Load() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/personalization-service/.env")

	brokersStr := getEnv("KAFKA_BROKERS", "localhost:9092")
	brokers := strings.Split(brokersStr, ",")

	return &Config{
		App: AppConfig{
			Name:        getEnv("APP_NAME", "personalization-service"),
			Environment: getEnv("APP_ENV", "development"),
			Version:     getEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: getEnv("HTTP_PORT", "8095"),
		},
		GRPC: GRPCConfig{
			Port: getEnv("GRPC_PORT", "50065"),
		},
		Database: DatabaseConfig{
			URL:            getEnv("PERSONALIZATION_SERVICE_DATABASE_URL", getEnv("DATABASE_URL", "")),
			MaxConns:       int32(getEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:       int32(getEnvAsInt("DB_MIN_CONNS", 2)),
			AutoMigrate:    getEnvAsBool("DB_AUTO_MIGRATE", true),
			MigrationsPath: getEnv("DB_MIGRATIONS_PATH", "./migrations"),
		},
		Kafka: KafkaConfig{
			Brokers:  brokers,
			ClientID: getEnv("KAFKA_CLIENT_ID", "personalization-service"),
			GroupID:  getEnv("KAFKA_GROUP_ID", "gocart.personalization-service.activity-group"),
			Enabled:  getEnvAsBool("KAFKA_ENABLED", true),
		},
		Logger: LoggerConfig{
			Level:  getEnv("LOG_LEVEL", "debug"),
			Format: getEnv("LOG_FORMAT", "json"),
		},
		Tracing: TracingConfig{
			Enabled:      getEnvAsBool("TRACING_ENABLED", true),
			Exporter:     getEnv("TRACING_EXPORTER", "stdout"),
			OTLPEndpoint: getEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:4317"),
		},
	}
}

func getEnv(key, defaultVal string) string {
	if val := os.Getenv(key); val != "" {
		return val
	}
	return defaultVal
}

func getEnvAsInt(key string, defaultVal int) int {
	if val := os.Getenv(key); val != "" {
		i, err := strconv.Atoi(val)
		if err == nil {
			return i
		}
	}
	return defaultVal
}

func getEnvAsBool(key string, defaultVal bool) bool {
	if val := os.Getenv(key); val != "" {
		b, err := strconv.ParseBool(val)
		if err == nil {
			return b
		}
	}
	return defaultVal
}