package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
)

type Config struct {
	App                         AppConfig
	HTTP                        HTTPConfig
	Services                    ServicesConfig
	Logger                      LoggerConfig
	Tracing                     TracingConfig
	GraphQL                     GraphQLConfig
	JWT                         JWTConfig
	UserServiceAddr             string
	ProductServiceAddr          string
	CartServiceAddr             string
	OrderServiceAddr            string
	GraphQLIntrospectionEnabled bool
}

type AppConfig struct {
	Name        string
	Environment string
	Version     string
}

type HTTPConfig struct {
	Port string
}

type ServicesConfig struct {
	UserServiceAddr    string
	ProductServiceAddr string
	CartServiceAddr    string
	OrderServiceAddr   string
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

type GraphQLConfig struct {
	PlaygroundEnabled bool
}

type JWTConfig struct {
	Secret string
	Issuer string
}

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("gateway/api-gateway/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	userServiceAddr := GetEnv("USER_SERVICE_ADDR", "localhost:5050")
	productServiceAddr := GetEnv("PRODUCT_SERVICE_ADDR", "localhost:7070")
	cartServiceAddr := GetEnv("CART_SERVICE_ADDR", "localhost:8181")
	orderServiceAddr := GetEnv("ORDER_SERVICE_ADDR", "localhost:8500")
	introEnabled := GetEnvAsBool("GRAPHQL_INTROSPECTION_ENABLED", GetEnvAsBool("GRAPHQL_PLAYGROUND_ENABLED", true))

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "api-gateway"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "8080"),
		},
		Services: ServicesConfig{
			UserServiceAddr:    userServiceAddr,
			ProductServiceAddr: productServiceAddr,
			CartServiceAddr:    cartServiceAddr,
			OrderServiceAddr:   orderServiceAddr,
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
		GraphQL: GraphQLConfig{
			PlaygroundEnabled: GetEnvAsBool("GRAPHQL_PLAYGROUND_ENABLED", true),
		},
		JWT: JWTConfig{
			Secret: GetEnv("JWT_SECRET", "gocart-secret-key-change-in-production"),
			Issuer: GetEnv("JWT_ISSUER", "gocart-api-gateway"),
		},
		UserServiceAddr:             userServiceAddr,
		ProductServiceAddr:          productServiceAddr,
		CartServiceAddr:             cartServiceAddr,
		OrderServiceAddr:            orderServiceAddr,
		GraphQLIntrospectionEnabled: introEnabled,
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
