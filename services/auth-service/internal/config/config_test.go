package config

import (
	"os"
	"testing"
)

func TestLoadEnv(t *testing.T) {
	_ = os.Setenv("PORT", "9999")
	_ = os.Setenv("APP_ENV", "test")
	_ = os.Setenv("ENV", "test")
	_ = os.Setenv("DB_MAX_CONNS", "10")
	_ = os.Setenv("DB_AUTO_MIGRATE", "false")
	_ = os.Setenv("JWT_SECRET", "test-secret")
	_ = os.Setenv("JWT_EXPIRY_MINUTES", "120")

	defer func() {
		_ = os.Unsetenv("PORT")
		_ = os.Unsetenv("APP_ENV")
		_ = os.Unsetenv("ENV")
		_ = os.Unsetenv("DB_MAX_CONNS")
		_ = os.Unsetenv("DB_AUTO_MIGRATE")
		_ = os.Unsetenv("JWT_SECRET")
		_ = os.Unsetenv("JWT_EXPIRY_MINUTES")
	}()

	cfg := LoadEnv()

	if cfg.HTTP.Port != "9999" {
		t.Fatalf("expected port '9999', got '%s'", cfg.HTTP.Port)
	}
	if cfg.App.Environment != "test" {
		t.Fatalf("expected env 'test', got '%s'", cfg.App.Environment)
	}
	if cfg.Database.MaxConns != 10 {
		t.Fatalf("expected max conns 10, got %d", cfg.Database.MaxConns)
	}
	if cfg.Database.AutoMigrate != false {
		t.Fatalf("expected auto migrate false, got %v", cfg.Database.AutoMigrate)
	}
	if cfg.JWT.Secret != "test-secret" {
		t.Fatalf("expected jwt secret 'test-secret', got '%s'", cfg.JWT.Secret)
	}
	if cfg.JWT.ExpiryMinutes != 120 {
		t.Fatalf("expected jwt expiry 120, got %d", cfg.JWT.ExpiryMinutes)
	}
}

func TestGetEnvHelpers(t *testing.T) {
	if got := GetEnv("NON_EXISTENT_KEY", "fallback"); got != "fallback" {
		t.Errorf("expected fallback, got %s", got)
	}
	if got := GetEnvAsInt("NON_EXISTENT_INT", 42); got != 42 {
		t.Errorf("expected 42, got %d", got)
	}
	if got := GetEnvAsBool("NON_EXISTENT_BOOL", true); got != true {
		t.Errorf("expected true, got %v", got)
	}
}
