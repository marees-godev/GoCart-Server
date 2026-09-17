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
	if cfg.UserServiceAddr == "" {
		t.Errorf("expected default UserServiceAddr, got empty")
	}
	if cfg.ProductServiceAddr == "" {
		t.Errorf("expected default ProductServiceAddr, got empty")
	}
	if cfg.CartServiceAddr == "" {
		t.Errorf("expected default CartServiceAddr, got empty")
	}
	if cfg.OrderServiceAddr == "" {
		t.Errorf("expected default OrderServiceAddr, got empty")
	}
	if !cfg.GraphQLIntrospectionEnabled {
		t.Errorf("expected default GraphQLIntrospectionEnabled to be true")
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
