package config

import (
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type Config struct {
	App         AppConfig
	HTTP        HTTPConfig
	GRPC        GRPCConfig
	Services    ServicesConfig
	Database    DatabaseConfig
	Reservation ReservationConfig
	Kafka       KafkaConfig
	Logger      LoggerConfig
	Tracing     TracingConfig
}

type KafkaConfig struct {
	Brokers []string
	Enabled bool
}

type ReservationConfig struct {
	ExpirationCleanupIntervalSeconds int
	DefaultExpirationMinutes         int
}

type GRPCConfig struct {
	Port string
}

type ServicesConfig struct {
	ProductServiceAddr string
	StoreServiceAddr   string
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

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/inventory-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "inventory-service"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "8300"),
		},
		GRPC: GRPCConfig{
			Port: GetEnv("GRPC_PORT", "50058"),
		},
		Services: ServicesConfig{
			ProductServiceAddr: GetEnv("PRODUCT_SERVICE_GRPC_ADDR", "localhost:50053"),
			StoreServiceAddr:   GetEnv("STORE_SERVICE_GRPC_ADDR", "localhost:50055"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("INVENTORY_SERVICE_DATABASE_URL", ""),
			MaxConns:       int32(GetEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:       int32(GetEnvAsInt("DB_MIN_CONNS", 2)),
			AutoMigrate:    GetEnvAsBool("DB_AUTO_MIGRATE", true),
			MigrationsPath: GetEnv("DB_MIGRATIONS_PATH", "./migrations"),
		},
		Reservation: ReservationConfig{
			ExpirationCleanupIntervalSeconds: GetEnvAsInt("RESERVATION_EXPIRATION_CLEANUP_INTERVAL_SECONDS", 30),
			DefaultExpirationMinutes:         GetEnvAsInt("DEFAULT_RESERVATION_EXPIRATION_MINUTES", 15),
		},
		Kafka: KafkaConfig{
			Brokers: strings.Split(GetEnv("KAFKA_BROKERS", "localhost:9092"), ","),
			Enabled: GetEnvAsBool("KAFKA_ENABLED", true),
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
