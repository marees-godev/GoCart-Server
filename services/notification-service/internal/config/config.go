package config

import (
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/provider"
)

type Config struct {
	App       AppConfig
	HTTP      HTTPConfig
	Database  DatabaseConfig
	Logger    LoggerConfig
	Tracing   TracingConfig
	Kafka     KafkaConfig
	Email     provider.EmailConfig
	Providers provider.ProviderConfig
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
	GroupID  string
	Enabled  bool
}

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/notification-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

		emailCfg := provider.EmailConfig{
			Provider:        GetEnv("EMAIL_PROVIDER", "resend_brevo"),
			ResendAPIKey:    GetEnv("RESEND_API_KEY", ""),
			ResendFromEmail: GetEnv("RESEND_FROM_EMAIL", GetEnv("EMAIL_FROM", "onboarding@resend.dev")),
			BrevoAPIKey:     GetEnv("BREVO_API_KEY", ""),
			BrevoFromEmail:  GetEnv("BREVO_FROM_EMAIL", GetEnv("EMAIL_FROM", "nikotest122@gmail.com")),
			FromEmail:       GetEnv("EMAIL_FROM", "onboarding@resend.dev"),
			Timeout:         GetEnvAsDuration("EMAIL_TIMEOUT", 10*time.Second),
		}

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "notification-service"),
			Environment: GetEnv("ENV", GetEnv("APP_ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "9099"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("NOTIFICATIONS_SERVICE_DATABASE_URL", ""),
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
			ClientID: GetEnv("KAFKA_CLIENT_ID", "notification-service"),
			GroupID:  GetEnv("KAFKA_GROUP_ID", "gocart.notification-service.domain-events-group"),
			Enabled:  GetEnvAsBool("KAFKA_ENABLED", true),
		},
		Email: emailCfg,
		Providers: provider.ProviderConfig{
			Email: emailCfg,
		},
	}
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
