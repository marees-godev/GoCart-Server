package redis

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/health"
)

func TestConfigDefaults(t *testing.T) {
	cfg := LoadConfigFromEnv("TEST_DEFAULT_PREFIX")
	if cfg.Host != "localhost" {
		t.Errorf("expected host localhost, got %s", cfg.Host)
	}
	if cfg.Port != "6379" {
		t.Errorf("expected port 6379, got %s", cfg.Port)
	}
	if cfg.Addr() != "localhost:6379" {
		t.Errorf("expected addr localhost:6379, got %s", cfg.Addr())
	}
	if cfg.PoolSize != 20 {
		t.Errorf("expected pool size 20, got %d", cfg.PoolSize)
	}
}

func TestLoadConfigFromEnv(t *testing.T) {
	_ = os.Setenv("REDIS_HOST", "redis.local")
	_ = os.Setenv("REDIS_PORT", "6380")
	_ = os.Setenv("REDIS_PASSWORD", "secret123")
	_ = os.Setenv("REDIS_DB", "2")
	_ = os.Setenv("REDIS_POOL_SIZE", "50")
	_ = os.Setenv("CART_REDIS_DB", "5")
	defer func() {
		_ = os.Unsetenv("REDIS_HOST")
		_ = os.Unsetenv("REDIS_PORT")
		_ = os.Unsetenv("REDIS_PASSWORD")
		_ = os.Unsetenv("REDIS_DB")
		_ = os.Unsetenv("REDIS_POOL_SIZE")
		_ = os.Unsetenv("CART_REDIS_DB")
	}()

	baseCfg := LoadConfigFromEnv("")
	if baseCfg.Host != "redis.local" || baseCfg.Port != "6380" || baseCfg.DB != 2 || baseCfg.PoolSize != 50 {
		t.Errorf("unexpected base config: %+v", baseCfg)
	}

	cartCfg := LoadConfigFromEnv("cart")
	if cartCfg.DB != 5 {
		t.Errorf("expected cart prefix to override DB to 5, got %d", cartCfg.DB)
	}
	if cartCfg.Host != "redis.local" {
		t.Errorf("expected fallback to general REDIS_HOST, got %s", cartCfg.Host)
	}
}

func TestSanitizeURL(t *testing.T) {
	raw := "redis://:supersecretpass@redis-host:6379/1"
	sanitized := SanitizeURL(raw)
	if sanitized == raw || sanitized == "" {
		t.Errorf("expected URL to be redacted, got %s", sanitized)
	}

	sanitizedAddr := SanitizeAddr("127.0.0.1:6379", "secret")
	if sanitizedAddr != "127.0.0.1:6379 (password protected)" {
		t.Errorf("unexpected sanitized addr: %s", sanitizedAddr)
	}
}

func TestHealthPingerCompatibility(t *testing.T) {
	// Compile-time interface check
	var _ health.Pinger = (*Client)(nil)

	// Runtime check on nil client
	var c *Client
	if err := c.Ping(context.Background()); err == nil {
		t.Fatal("expected error on nil client Ping")
	}
}

func TestNilClientSafety(t *testing.T) {
	var c *Client
	ctx := context.Background()

	if _, err := c.Get(ctx, "test"); err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
	if err := c.Set(ctx, "test", "value", time.Minute); err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
	if err := c.Delete(ctx, "test"); err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
	if _, err := c.Exists(ctx, "test"); err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
	if _, err := c.SetNX(ctx, "test", "v", time.Minute); err != ErrNilClient {
		t.Errorf("expected ErrNilClient, got %v", err)
	}
	if err := c.Close(); err != nil {
		t.Errorf("expected nil error on close of nil client, got %v", err)
	}
}

func TestConnectionFailureHandledWithoutCrash(t *testing.T) {
	cfg := Config{
		Host:          "127.0.0.1",
		Port:          "65534", // Unreachable port
		MaxRetries:    2,
		RetryInterval: 50 * time.Millisecond,
		DialTimeout:   100 * time.Millisecond,
	}

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	client, err := New(ctx, cfg)
	if err == nil {
		t.Fatal("expected connection error for unreachable port")
	}
	if client != nil {
		t.Fatal("expected nil client on failed connection")
	}
}
