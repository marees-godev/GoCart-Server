package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	GRPC     GRPCConfig
	Database DatabaseConfig
	Logger   LoggerConfig
	Tracing  TracingConfig
	Storage  StorageConfig
	GSTIN    GSTINConfig
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

type LoggerConfig struct {
	Level  string
	Format string
}

type TracingConfig struct {
	Enabled      bool
	Exporter     string
	OTLPEndpoint string
}

type StorageConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURLPrefix string
}

type GSTINConfig struct {
	APIKey  string
	BaseURL string
	Enabled bool
	Timeout time.Duration
}

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/store-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	timeoutSec := GetEnvAsInt("GSTIN_API_TIMEOUT_SECONDS", 10)

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "store-service"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "7500"),
		},
		GRPC: GRPCConfig{
			Port: GetEnv("GRPC_PORT", "50055"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("STORE_SERVICE_DATABASE_URL", ""),
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
		Storage: StorageConfig{
			Endpoint:        GetEnv("STORE_S3_ENDPOINT", GetEnv("S3_ENDPOINT", "https://cljkfzbiywvhzpmlbbuy.storage.supabase.co/storage/v1/s3")),
			Region:          GetEnv("STORE_S3_REGION", GetEnv("S3_REGION", "ap-south-1")),
			AccessKeyID:     GetEnv("STORE_S3_ACCESS_KEY_ID", GetEnv("S3_ACCESS_KEY_ID", "")),
			SecretAccessKey: GetEnv("STORE_S3_SECRET_ACCESS_KEY", GetEnv("S3_SECRET_ACCESS_KEY", "")),
			Bucket:          GetEnv("STORE_S3_BUCKET", GetEnv("S3_BUCKET", "stores")),
			PublicURLPrefix: GetEnv("STORE_S3_PUBLIC_URL_PREFIX", GetEnv("S3_PUBLIC_URL_PREFIX", "")),
		},
		GSTIN: GSTINConfig{
			APIKey:  GetEnv("GSTIN_API_KEY", ""),
			BaseURL: GetEnv("GSTIN_API_BASE_URL", "https://www.gstinapi.in/v1"),
			Enabled: GetEnvAsBool("GSTIN_VERIFICATION_ENABLED", true),
			Timeout: time.Duration(timeoutSec) * time.Second,
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
