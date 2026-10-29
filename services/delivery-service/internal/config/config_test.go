package config_test

import (
	"os"
	"testing"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/config"
)

func TestConfig_StartupValidation(t *testing.T) {
	t.Run("Deployed environment fails when Database URL is empty", func(t *testing.T) {
		cfg := &config.Config{
			App:      config.AppConfig{Environment: "production"},
			Database: config.DatabaseConfig{URL: ""},
		}
		if err := cfg.Validate(); err == nil {
			t.Fatal("expected error for empty database URL in production, got nil")
		}
	})

	t.Run("Deployed environment passes with valid Database URL", func(t *testing.T) {
		cfg := &config.Config{
			App:      config.AppConfig{Environment: "production"},
			Database: config.DatabaseConfig{URL: "postgres://user:pass@localhost:5432/delivery_db"},
		}
		if err := cfg.Validate(); err != nil {
			t.Fatalf("expected nil error for valid database URL, got: %v", err)
		}
	})

	t.Run("Development environment allows startup validation", func(t *testing.T) {
		cfgDev := &config.Config{
			App: config.AppConfig{Environment: "development"},
		}
		if err := cfgDev.Validate(); err != nil {
			t.Fatalf("expected development environment to pass validation, got: %v", err)
		}
	})
}

func TestConfig_LoadEnv(t *testing.T) {
	origPort := os.Getenv("PORT")
	defer func() {
		if origPort != "" {
			_ = os.Setenv("PORT", origPort)
		} else {
			_ = os.Unsetenv("PORT")
		}
	}()

	_ = os.Setenv("PORT", "9955")
	cfg := config.LoadEnv()
	if cfg.HTTP.Port != "9955" {
		t.Fatalf("expected HTTP.Port to be '9955', got %q", cfg.HTTP.Port)
	}
}
