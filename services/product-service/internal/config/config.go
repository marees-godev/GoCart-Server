package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type StorageConfig struct {
	Endpoint        string
	Region          string
	AccessKeyID     string
	SecretAccessKey string
	Bucket          string
	PublicURLPrefix string
}

type Config struct {
	App      AppConfig
	HTTP     HTTPConfig
	GRPC     GRPCConfig
	Database DatabaseConfig
	Storage  StorageConfig
	Logger   LoggerConfig
	Tracing  TracingConfig
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
	Port                string
	StoreServiceAddr    string
	CategoryServiceAddr string
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
	_ = godotenv.Load("services/product-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "product-service"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "7070"),
		},
		GRPC: GRPCConfig{
			Port:                GetEnv("PRODUCT_SERVICE_GRPC_PORT", GetEnv("GRPC_PORT", "50053")),
			StoreServiceAddr:    GetEnv("STORE_SERVICE_GRPC_ADDR", "localhost:50055"),
			CategoryServiceAddr: GetEnv("CATEGORY_SERVICE_GRPC_ADDR", "localhost:50054"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("PRODUCT_SERVICE_DATABASE_URL", GetEnv("DATABASE_URL", "")),
			MaxConns:       int32(GetEnvAsInt("DB_MAX_CONNS", 25)),
			MinConns:       int32(GetEnvAsInt("DB_MIN_CONNS", 2)),
			AutoMigrate:    GetEnvAsBool("DB_AUTO_MIGRATE", true),
			MigrationsPath: GetEnv("DB_MIGRATIONS_PATH", "./migrations"),
		},
		Storage: StorageConfig{
			Endpoint:        GetEnv("PRODUCT_S3_ENDPOINT", GetEnv("S3_ENDPOINT", "https://cljkfzbiywvhzpmlbbuy.storage.supabase.co/storage/v1/s3")),
			Region:          GetEnv("PRODUCT_S3_REGION", GetEnv("S3_REGION", "ap-south-1")),
			AccessKeyID:     GetEnv("PRODUCT_S3_ACCESS_KEY_ID", GetEnv("S3_ACCESS_KEY_ID", "")),
			SecretAccessKey: GetEnv("PRODUCT_S3_SECRET_ACCESS_KEY", GetEnv("S3_SECRET_ACCESS_KEY", "")),
			Bucket:          GetEnv("PRODUCT_S3_BUCKET", GetEnv("S3_BUCKET", "products")),
			PublicURLPrefix: GetEnv("PRODUCT_S3_PUBLIC_URL_PREFIX", GetEnv("S3_PUBLIC_URL_PREFIX", "")),
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
