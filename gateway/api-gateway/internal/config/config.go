package config

import (
	"os"
	"strconv"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	App                         AppConfig
	HTTP                        HTTPConfig
	Services                    ServicesConfig
	Logger                      LoggerConfig
	Tracing                     TracingConfig
	GraphQL                     GraphQLConfig
	GRPC                        GRPCConfig
	JWT                         JWTConfig
	RateLimit                   RateLimitConfig
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
	PlaygroundEnabled    bool
	IntrospectionEnabled bool
}

type GRPCConfig struct {
	DefaultTimeout          time.Duration
	AuthServiceAddr         string
	UserServiceAddr         string
	ProductServiceAddr      string
	CategoryServiceAddr     string
	StoreServiceAddr        string
	MerchantServiceAddr     string
	CartServiceAddr         string
	InventoryServiceAddr    string
	OrderServiceAddr        string
	PaymentServiceAddr      string
	DeliveryServiceAddr     string
	ReturnServiceAddr       string
	RatingServiceAddr       string
	NotificationServiceAddr string
}

type JWTConfig struct {
	Secret string
	Issuer string
}

type RateLimitConfig struct {
	Enabled bool
	Max     int
	Window  time.Duration
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
			PlaygroundEnabled:    GetEnvAsBool("GRAPHQL_PLAYGROUND_ENABLED", true),
			IntrospectionEnabled: introEnabled,
		},
		GRPC: GRPCConfig{
			DefaultTimeout:          GetEnvAsDuration("GRPC_DEFAULT_TIMEOUT", 5*time.Second),
			AuthServiceAddr:         GetEnv("AUTH_SERVICE_GRPC_ADDR", "localhost:50051"),
			UserServiceAddr:         GetEnv("USER_SERVICE_GRPC_ADDR", "localhost:50052"),
			ProductServiceAddr:      GetEnv("PRODUCT_SERVICE_GRPC_ADDR", "localhost:50053"),
			CategoryServiceAddr:     GetEnv("CATEGORY_SERVICE_GRPC_ADDR", "localhost:50054"),
			StoreServiceAddr:        GetEnv("STORE_SERVICE_GRPC_ADDR", "localhost:50055"),
			MerchantServiceAddr:     GetEnv("MERCHANT_SERVICE_GRPC_ADDR", "localhost:50056"),
			CartServiceAddr:         GetEnv("CART_SERVICE_GRPC_ADDR", "localhost:50057"),
			InventoryServiceAddr:    GetEnv("INVENTORY_SERVICE_GRPC_ADDR", "localhost:50058"),
			OrderServiceAddr:        GetEnv("ORDER_SERVICE_GRPC_ADDR", "localhost:50059"),
			PaymentServiceAddr:      GetEnv("PAYMENT_SERVICE_GRPC_ADDR", "localhost:50060"),
			DeliveryServiceAddr:     GetEnv("DELIVERY_SERVICE_GRPC_ADDR", "localhost:50061"),
			ReturnServiceAddr:       GetEnv("RETURN_SERVICE_GRPC_ADDR", "localhost:50062"),
			RatingServiceAddr:       GetEnv("RATING_SERVICE_GRPC_ADDR", "localhost:50063"),
			NotificationServiceAddr: GetEnv("NOTIFICATION_SERVICE_GRPC_ADDR", "localhost:50064"),
		},
		JWT: JWTConfig{
			Secret: GetEnv("JWT_SECRET", "gocart-secret-key-change-in-production"),
			Issuer: GetEnv("JWT_ISSUER", "gocart-api-gateway"),
		},
		RateLimit: RateLimitConfig{
			Enabled: GetEnvAsBool("RATE_LIMIT_ENABLED", true),
			Max:     GetEnvAsInt("RATE_LIMIT_MAX_REQUESTS", 1000),
			Window:  GetEnvAsDuration("RATE_LIMIT_WINDOW", time.Minute),
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
