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
	GRPC     GRPCConfig
	Database  DatabaseConfig
	Logger    LoggerConfig
	Tracing   TracingConfig
	Kafka     KafkaConfig
	Email     provider.EmailConfig
	Providers provider.ProviderConfig
	VAPID    VAPIDConfig
	Push     PushConfig
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
	Port             string
	OrderServiceAddr string
	UserServiceAddr  string
	LookupTimeout    time.Duration
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
	Brokers       []string
	ClientID      string
	GroupID       string
	Topics        []string
	Enabled       bool
	MaxRetries    int
	RetryInterval time.Duration
	DLQSuffix     string
}

type VAPIDConfig struct {
	PublicKey  string
	PrivateKey string
	Subject    string
}

func (v VAPIDConfig) IsConfigured() bool {
	return strings.TrimSpace(v.PublicKey) != "" && strings.TrimSpace(v.PrivateKey) != ""
}

type PushConfig struct {
	RequestTimeout time.Duration
	MaxRetries     int
	RetryInterval  time.Duration
	TTLSeconds     int
	LocalNotify    bool
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

	dbURL := GetEnv("NOTIFICATIONS_SERVICE_DATABASE_URL", "")
	if dbURL == "" {
		dbURL = GetEnv("NOTIFICATION_SERVICE_DATABASE_URL", "")
	}
	if dbURL == "" {
		dbURL = GetEnv("DATABASE_URL", "")
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
		GRPC: GRPCConfig{
			Port:             GetEnv("GRPC_PORT", "50064"),
			OrderServiceAddr: GetEnv("ORDER_SERVICE_GRPC_ADDR", "localhost:50059"),
			UserServiceAddr:  GetEnv("USER_SERVICE_GRPC_ADDR", "localhost:50052"),
			LookupTimeout:    GetEnvAsDuration("GRPC_LOOKUP_TIMEOUT", 2*time.Second),
		},
		Database: DatabaseConfig{
			URL:            dbURL,
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
			Brokers:       GetEnvAsStringSlice("KAFKA_BROKERS", []string{"localhost:9092"}),
			ClientID:      GetEnv("KAFKA_CLIENT_ID", "notification-service"),
			GroupID:       GetEnv("KAFKA_GROUP_ID", "gocart.notification-service.domain-events-group"),
			Topics:        GetEnvAsStringSlice("KAFKA_TOPICS", nil),
			Enabled:       GetEnvAsBool("KAFKA_ENABLED", true),
			MaxRetries:    GetEnvAsInt("KAFKA_MAX_RETRIES", 3),
			RetryInterval: GetEnvAsDuration("KAFKA_RETRY_INTERVAL", 500*time.Millisecond),
			DLQSuffix:     GetEnv("KAFKA_DLQ_SUFFIX", ".dlq"),
		},
		VAPID: VAPIDConfig{
			PublicKey:  GetEnv("VAPID_PUBLIC_KEY", ""),
			PrivateKey: GetEnv("VAPID_PRIVATE_KEY", ""),
			Subject:    GetEnv("VAPID_SUBJECT", "mailto:notifications@gocart.local"),
		},
		Push: PushConfig{
			RequestTimeout: GetEnvAsDuration("PUSH_REQUEST_TIMEOUT", 10*time.Second),
			MaxRetries:     GetEnvAsInt("PUSH_MAX_RETRIES", 3),
			RetryInterval:  GetEnvAsDuration("PUSH_RETRY_INTERVAL", 500*time.Millisecond),
			TTLSeconds:     GetEnvAsInt("PUSH_TTL_SECONDS", 86400),
			LocalNotify:    GetEnvAsBool("PUSH_LOCAL_NOTIFICATIONS_ENABLED", true),
		},
		Email: emailCfg,
		Providers: provider.ProviderConfig{
			Email: emailCfg,
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
	res := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			res = append(res, trimmed)
		}
	}
	return res
}
