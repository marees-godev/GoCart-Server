package database

import (
	"context"
	"os"
	"testing"
	"time"
)

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if cfg.MaxConns != 25 {
		t.Errorf("expected MaxConns=25, got %d", cfg.MaxConns)
	}
	if cfg.MinConns != 2 {
		t.Errorf("expected MinConns=2, got %d", cfg.MinConns)
	}
	if cfg.MaxConnLifetime != 1*time.Hour {
		t.Errorf("expected MaxConnLifetime=1h, got %v", cfg.MaxConnLifetime)
	}
	if cfg.MaxConnIdleTime != 30*time.Minute {
		t.Errorf("expected MaxConnIdleTime=30m, got %v", cfg.MaxConnIdleTime)
	}
	if cfg.MaxRetries != 3 {
		t.Errorf("expected MaxRetries=3, got %d", cfg.MaxRetries)
	}
	if cfg.AutoMigrate != false {
		t.Errorf("expected AutoMigrate=false, got %v", cfg.AutoMigrate)
	}
}

func TestLoadConfigFromEnv_WithPrefix(t *testing.T) {
	os.Setenv("AUTH_DATABASE_URL", "postgresql://user:secret@localhost:5432/auth_db")
	os.Setenv("AUTH_DB_MAX_CONns", "50")
	os.Setenv("AUTH_DB_MIN_CONNS", "5")
	os.Setenv("AUTH_DB_AUTO_MIGRATE", "true")
	defer func() {
		os.Unsetenv("AUTH_DATABASE_URL")
		os.Unsetenv("AUTH_DB_MAX_CONNS")
		os.Unsetenv("AUTH_DB_MIN_CONNS")
		os.Unsetenv("AUTH_DB_AUTO_MIGRATE")
	}()

	cfg, err := LoadConfigFromEnv("AUTH")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if cfg.URL != "postgresql://user:secret@localhost:5432/auth_db" {
		t.Errorf("unexpected URL: %s", cfg.URL)
	}
	if cfg.MinConns != 5 {
		t.Errorf("expected MinConns=5, got %d", cfg.MinConns)
	}
	if !cfg.AutoMigrate {
		t.Errorf("expected AutoMigrate=true, got %v", cfg.AutoMigrate)
	}
}

func TestLoadConfigFromEnv_MissingURL(t *testing.T) {
	os.Unsetenv("CUSTOM_DATABASE_URL")
	os.Unsetenv("DATABASE_URL")

	_, err := LoadConfigFromEnv("CUSTOM")
	if err == nil {
		t.Fatal("expected error for missing database URL, got nil")
	}
}

func TestSanitizeURL(t *testing.T) {
	raw := "postgresql://postgres:MySuperSecretPassword@db.supabase.co:5432/postgres?sslmode=require"
	sanitized := SanitizeURL(raw)

	if sanitized == raw {
		t.Fatal("SanitizeURL failed to mask password")
	}
	if sanitized != "postgresql://postgres:xxxxx@db.supabase.co:5432/postgres?sslmode=require" {
		t.Errorf("unexpected sanitized URL output: %s", sanitized)
	}
}

func TestSanitizeURL_Invalid(t *testing.T) {
	raw := "::not a valid url"
	sanitized := SanitizeURL(raw)
	if sanitized != "[malformed database url]" {
		t.Errorf("expected [malformed database url], got %s", sanitized)
	}
}

func TestNew_EmptyURL(t *testing.T) {
	cfg := Config{}
	_, err := New(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected error with empty URL, got nil")
	}
}
