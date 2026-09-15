package database

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	URL               string
	MaxConns          int32
	MinConns          int32
	MaxConnLifetime   time.Duration
	MaxConnIdleTime   time.Duration
	HealthCheckPeriod time.Duration
	ConnectTimeout    time.Duration
	MaxRetries        int
	RetryInterval     time.Duration
	AutoMigrate       bool
	MigrationsPath    string
}

func DefaultConfig() Config {
	return Config{
		MaxConns:          25,
		MinConns:          2,
		MaxConnLifetime:   1 * time.Hour,
		MaxConnIdleTime:   30 * time.Minute,
		HealthCheckPeriod: 1 * time.Minute,
		ConnectTimeout:    10 * time.Second,
		MaxRetries:        3,
		RetryInterval:     1 * time.Second,
		AutoMigrate:       false,
		MigrationsPath:    "./migrations",
	}
}

func LoadConfigFromEnv(servicePrefix string) (Config, error) {
	cfg := DefaultConfig()

	var url string
	if servicePrefix != "" {
		url = os.Getenv(fmt.Sprintf("%s_DATABASE_URL", servicePrefix))
	}
	if url == "" {
		url = os.Getenv("DATABASE_URL")
	}
	if url == "" {
		return cfg, fmt.Errorf("database URL is required")
	}
	cfg.URL = url

	if val := getEnvWithPrefix(servicePrefix, "DB_MAX_CONNS"); val != "" {
		if n, err := strconv.ParseInt(val, 10, 32); err == nil && n > 0 {
			cfg.MaxConns = int32(n)
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_MIN_CONNS"); val != "" {
		if n, err := strconv.ParseInt(val, 10, 32); err == nil && n >= 0 {
			cfg.MinConns = int32(n)
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_MAX_CONN_LIFETIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MaxConnLifetime = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_MAX_CONN_IDLE_TIME"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.MaxConnIdleTime = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_CONNECT_TIMEOUT"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.ConnectTimeout = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_MAX_RETRIES"); val != "" {
		if n, err := strconv.Atoi(val); err == nil && n >= 0 {
			cfg.MaxRetries = n
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_RETRY_INTERVAL"); val != "" {
		if d, err := time.ParseDuration(val); err == nil {
			cfg.RetryInterval = d
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_AUTO_MIGRATE"); val != "" {
		if b, err := strconv.ParseBool(val); err == nil {
			cfg.AutoMigrate = b
		}
	}

	if val := getEnvWithPrefix(servicePrefix, "DB_MIGRATIONS_PATH"); val != "" {
		cfg.MigrationsPath = val
	}

	return cfg, nil
}

func getEnvWithPrefix(prefix, key string) string {
	if prefix != "" {
		if val := os.Getenv(fmt.Sprintf("%s_%s", prefix, key)); val != "" {
			return val
		}
	}
	return os.Getenv(key)
}
