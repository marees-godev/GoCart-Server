package config

import (
	"os"
	"strconv"

	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/pkg/mailer"
	"github.com/marees-godev/GoCart-Server/pkg/redis"
)

type Config struct {
	App             AppConfig
	HTTP            HTTPConfig
	GRPC            GRPCConfig
	Database        DatabaseConfig
	Logger          LoggerConfig
	Tracing         TracingConfig
	JWT             JWTConfig
	Email           EmailConfig
	Redis           redis.Config
	UserServiceAddr string
	Services        ServicesConfig
}

type ServicesConfig struct {
	MerchantServiceURL string
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

type JWTConfig struct {
	Secret        string
	ExpiryMinutes int
}

type EmailConfig struct {
	ResendAPIKey    string
	ResendFromEmail string
	BrevoAPIKey     string
	BrevoFromEmail  string
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	FromEmail             string
	TokenTTLMinutes       int
	ResendCooldownSeconds int
}

func (c EmailConfig) ToMailerConfig() mailer.Config {
	return mailer.Config{
		ResendAPIKey:    c.ResendAPIKey,
		ResendFromEmail: c.ResendFromEmail,
		BrevoAPIKey:     c.BrevoAPIKey,
		BrevoFromEmail:  c.BrevoFromEmail,
		SMTPHost:        c.SMTPHost,
		SMTPPort:        c.SMTPPort,
		SMTPUser:        c.SMTPUser,
		SMTPPass:        c.SMTPPass,
		FromEmail:       c.FromEmail,
	}
}

func LoadEnv() *Config {
	_ = godotenv.Load(".env")
	_ = godotenv.Load("services/auth-service/.env")
	_ = godotenv.Load("../.env")
	_ = godotenv.Load("../../.env")

	return &Config{
		App: AppConfig{
			Name:        GetEnv("APP_NAME", "auth-service"),
			Environment: GetEnv("APP_ENV", GetEnv("ENV", "development")),
			Version:     GetEnv("APP_VERSION", "1.0.0"),
		},
		HTTP: HTTPConfig{
			Port: GetEnv("PORT", "9451"),
		},
		GRPC: GRPCConfig{
			Port: GetEnv("GRPC_PORT", "50051"),
		},
		Database: DatabaseConfig{
			URL:            GetEnv("AUTH_SERVICE_DATABASE_URL", ""),
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
		JWT: JWTConfig{
			Secret:        GetEnv("JWT_SECRET", "gocart-secret-key-change-in-production"),
			ExpiryMinutes: GetEnvAsInt("JWT_EXPIRY_MINUTES", 60),
		},
		Email: EmailConfig{
			ResendAPIKey:    GetEnv("RESEND_API_KEY", ""),
			ResendFromEmail: GetEnv("RESEND_FROM_EMAIL", GetEnv("EMAIL_FROM", "onboarding@resend.dev")),
			BrevoAPIKey:     GetEnv("BREVO_API_KEY", ""),
			BrevoFromEmail:  GetEnv("BREVO_FROM_EMAIL", GetEnv("EMAIL_FROM", "nikotest122@gmail.com")),
			SMTPHost:        GetEnv("SMTP_HOST", "smtp.gmail.com"),
			SMTPPort:        GetEnv("SMTP_PORT", "587"),
			SMTPUser:        GetEnv("SMTP_USER", ""),
			SMTPPass:        GetEnv("SMTP_PASS", ""),
			FromEmail:             GetEnv("EMAIL_FROM", "onboarding@resend.dev"),
			TokenTTLMinutes:       GetEnvAsInt("EMAIL_OTP_TTL_MINUTES", GetEnvAsInt("EMAIL_TOKEN_TTL_MINUTES", 5)),
			ResendCooldownSeconds: GetEnvAsInt("EMAIL_OTP_RESEND_COOLDOWN_SECONDS", 60),
		},
		Redis:           redis.LoadConfigFromEnv("AUTH"),
		UserServiceAddr: GetEnv("USER_SERVICE_GRPC_ADDR", GetEnv("USER_SERVICE_ADDR", "localhost:50052")),
		Services: ServicesConfig{
			MerchantServiceURL: GetEnv("MERCHANT_SERVICE_GRPC_URL", "localhost:50056"),
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
