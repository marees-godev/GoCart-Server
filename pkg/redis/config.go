package redis

import (
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/joho/godotenv"
)

type Config struct {
	URL           string
	Host          string
	Port          string
	Password      string
	DB            int
	PoolSize      int
	MinIdleConns  int
	DialTimeout   time.Duration
	ReadTimeout   time.Duration
	WriteTimeout  time.Duration
	MaxRetries    int
	RetryInterval time.Duration
	EnableTLS     bool
}

func (c Config) Addr() string {
	if c.Host == "" {
		c.Host = "localhost"
	}
	if c.Port == "" {
		c.Port = "6379"
	}
	return net.JoinHostPort(c.Host, c.Port)
}

func LoadConfigFromEnv(servicePrefix string) Config {
	loadEnv(servicePrefix)

	dialTimeout := getEnvAsDuration(servicePrefix, "REDIS_CONNECT_TIMEOUT", 5*time.Second)
	if dialStr := getRawEnv(servicePrefix, "REDIS_DIAL_TIMEOUT"); dialStr != "" {
		if d, err := time.ParseDuration(dialStr); err == nil {
			dialTimeout = d
		}
	}

	return Config{
		URL:           getEnv(servicePrefix, "REDIS_URL", ""),
		Host:          getEnv(servicePrefix, "REDIS_HOST", "localhost"),
		Port:          getEnv(servicePrefix, "REDIS_PORT", "6379"),
		Password:      getEnv(servicePrefix, "REDIS_PASSWORD", ""),
		DB:            getEnvAsInt(servicePrefix, "REDIS_DB", 0),
		PoolSize:      getEnvAsInt(servicePrefix, "REDIS_POOL_SIZE", 20),
		MinIdleConns:  getEnvAsInt(servicePrefix, "REDIS_MIN_IDLE_CONNS", 5),
		DialTimeout:   dialTimeout,
		ReadTimeout:   getEnvAsDuration(servicePrefix, "REDIS_READ_TIMEOUT", 3*time.Second),
		WriteTimeout:  getEnvAsDuration(servicePrefix, "REDIS_WRITE_TIMEOUT", 3*time.Second),
		MaxRetries:    getEnvAsInt(servicePrefix, "REDIS_MAX_RETRIES", 3),
		RetryInterval: getEnvAsDuration(servicePrefix, "REDIS_RETRY_INTERVAL", 1*time.Second),
		EnableTLS:     getEnvAsBool(servicePrefix, "REDIS_ENABLE_TLS", false),
	}
}

func loadEnv(servicePrefix string) {
	candidates := []string{
		".env",
		"../.env",
		"../../.env",
		".env.example",
		"../.env.example",
		"../../.env.example",
	}

	if servicePrefix != "" {
		sName := strings.ToLower(servicePrefix)
		candidates = append(candidates,
			fmt.Sprintf("services/%s-service/.env", sName),
			fmt.Sprintf("services/%s-service/.env.example", sName),
			fmt.Sprintf("gateway/%s/.env", sName),
			fmt.Sprintf("gateway/%s/.env.example", sName),
		)
	}

	for _, file := range candidates {
		if _, err := os.Stat(file); err == nil {
			if err := godotenv.Load(file); err != nil {
				slog.Warn("Failed to load environment file", "file", file, "error", err)
			}
		}
	}
}

func SanitizeURL(rawURL string) string {
	if rawURL == "" {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return "[malformed redis url]"
	}
	return u.Redacted()
}

func SanitizeAddr(addr, password string) string {
	if password != "" {
		return fmt.Sprintf("%s (password protected)", addr)
	}
	return addr
}

func getEnv(prefix, key, defaultValue string) string {
	if val := getRawEnv(prefix, key); val != "" {
		return val
	}
	return defaultValue
}

func getEnvAsInt(prefix, key string, defaultValue int) int {
	valStr := getRawEnv(prefix, key)
	if valStr == "" {
		return defaultValue
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return defaultValue
	}
	return val
}

func getEnvAsDuration(prefix, key string, defaultValue time.Duration) time.Duration {
	valStr := getRawEnv(prefix, key)
	if valStr == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(valStr)
	if err != nil {
		return defaultValue
	}
	return d
}

func getEnvAsBool(prefix, key string, defaultValue bool) bool {
	valStr := getRawEnv(prefix, key)
	if valStr == "" {
		return defaultValue
	}
	b, err := strconv.ParseBool(valStr)
	if err != nil {
		return defaultValue
	}
	return b
}

func getRawEnv(prefix, key string) string {
	if prefix != "" {
		upperPrefix := strings.ToUpper(prefix)
		if val := os.Getenv(fmt.Sprintf("%s_%s", upperPrefix, key)); val != "" {
			return val
		}
		if strings.HasPrefix(key, "REDIS_") {
			stripped := strings.TrimPrefix(key, "REDIS_")
			if val := os.Getenv(fmt.Sprintf("%s_%s", upperPrefix, stripped)); val != "" {
				return val
			}
		}
	}
	return os.Getenv(key)
}
