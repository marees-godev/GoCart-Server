package config

import (
	"os"
	"testing"
)

func TestLoadEnvDefaults(t *testing.T) {
	os.Unsetenv("APP_NAME")
	os.Unsetenv("PORT")
	os.Unsetenv("LOG_LEVEL")

	cfg := LoadEnv()

	if cfg.App.Name == "" {
		t.Errorf("expected default App.Name, got empty")
	}
	if cfg.HTTP.Port == "" {
		t.Errorf("expected default HTTP.Port, got empty")
	}
	if cfg.Logger.Level == "" {
		t.Errorf("expected default Logger.Level, got empty")
	}

}

func TestGetEnv(t *testing.T) {
	os.Setenv("TEST_GATEWAY_ENV_KEY", "custom_val")
	defer os.Unsetenv("TEST_GATEWAY_ENV_KEY")

	val := GetEnv("TEST_GATEWAY_ENV_KEY", "default_val")
	if val != "custom_val" {
		t.Errorf("expected 'custom_val', got '%s'", val)
	}

	fallback := GetEnv("NON_EXISTENT_ENV_KEY", "default_val")
	if fallback != "default_val" {
		t.Errorf("expected 'default_val', got '%s'", fallback)
	}
}

func TestGetEnvAsBool(t *testing.T) {
	os.Setenv("TEST_BOOL_KEY", "true")
	defer os.Unsetenv("TEST_BOOL_KEY")

	if !GetEnvAsBool("TEST_BOOL_KEY", false) {
		t.Errorf("expected true, got false")
	}

	if GetEnvAsBool("NON_EXISTENT_BOOL", false) {
		t.Errorf("expected default false, got true")
	}
}

func TestGetEnvAsDuration(t *testing.T) {
	os.Setenv("TEST_DURATION_KEY", "10s")
	defer os.Unsetenv("TEST_DURATION_KEY")

	if GetEnvAsDuration("TEST_DURATION_KEY", 0) != 10*1000000000 {
		t.Errorf("expected 10s duration")
	}

	if GetEnvAsDuration("NON_EXISTENT_DURATION", 5*1000000000) != 5*1000000000 {
		t.Errorf("expected default 5s duration")
	}
}


func TestConfig_Validate(t *testing.T) {
	t.Run("Deployed environment fails on missing or placeholder secret", func(t *testing.T) {
		cfgEmpty := &Config{
			App: AppConfig{Environment: "production"},
			JWT: JWTConfig{Secret: ""},
		}
		if err := cfgEmpty.Validate(); err == nil {
			t.Fatal("expected error for empty secret in production, got nil")
		}

		cfgPlaceholder := &Config{
			App: AppConfig{Environment: "production"},
			JWT: JWTConfig{Secret: "gocart-secret-key-change-in-production"},
		}
		if err := cfgPlaceholder.Validate(); err == nil {
			t.Fatal("expected error for placeholder secret in production, got nil")
		}
	})

	t.Run("Deployed environment passes with secure secret", func(t *testing.T) {
		cfgValid := &Config{
			App: AppConfig{Environment: "production"},
			JWT: JWTConfig{Secret: "production-strong-secret-key-998877"},
		}
		if err := cfgValid.Validate(); err != nil {
			t.Fatalf("expected nil error for valid secret, got: %v", err)
		}
	})
}
