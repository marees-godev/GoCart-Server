package logger

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
)

func TestStructuredJSONLogger(t *testing.T) {
	var buf bytes.Buffer
	cfg := Config{
		ServiceName: "test-service",
		Environment: "staging",
		Version:     "v1.0.0",
		Level:       "DEBUG",
		Format:      "json",
		Output:      &buf,
	}

	l := New(cfg)

	ctx := context.Background()
	ctx = WithRequestID(ctx, "req-12345")
	ctx = WithCorrelationID(ctx, "corr-67890")
	ctx = WithUserID(ctx, "user-999")

	l.InfoContext(ctx, "User registration initiated",
		slog.String("email", "john@example.com"),
		slog.String("password", "supersecret123"),
		slog.String("token", "bearer eyJhbGciOi..."),
	)

	var logEntry map[string]any
	if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
		t.Fatalf("Failed to parse log JSON: %v, raw: %s", err, buf.String())
	}

	if logEntry["service"] != "test-service" {
		t.Errorf("Expected service 'test-service', got '%v'", logEntry["service"])
	}
	if logEntry["environment"] != "staging" {
		t.Errorf("Expected environment 'staging', got '%v'", logEntry["environment"])
	}
	if logEntry["request_id"] != "req-12345" {
		t.Errorf("Expected request_id 'req-12345', got '%v'", logEntry["request_id"])
	}
	if logEntry["correlation_id"] != "corr-67890" {
		t.Errorf("Expected correlation_id 'corr-67890', got '%v'", logEntry["correlation_id"])
	}
	if logEntry["user_id"] != "user-999" {
		t.Errorf("Expected user_id 'user-999', got '%v'", logEntry["user_id"])
	}

	if logEntry["password"] != "[REDACTED]" {
		t.Errorf("Expected password to be '[REDACTED]', got '%v'", logEntry["password"])
	}
	if logEntry["token"] != "[REDACTED]" {
		t.Errorf("Expected token to be '[REDACTED]', got '%v'", logEntry["token"])
	}
}

func TestRedactSensitiveKeys(t *testing.T) {
	testCases := []struct {
		key      string
		val      string
		expected string
	}{
		{"password", "my-secret-pass", "[REDACTED]"},
		{"password_hash", "$2a$12$...", "[REDACTED]"},
		{"new_password", "new-secret-123", "[REDACTED]"},
		{"access_token", "jwt.token.here", "[REDACTED]"},
		{"refresh_token", "refresh-token-xyz", "[REDACTED]"},
		{"jwt", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.doNotLeakThisSignature", "[REDACTED]"},
		{"credit_card", "4111222233334444", "[REDACTED]"},
		{"card_number", "4111222233334444", "[REDACTED]"},
		{"cvv", "123", "[REDACTED]"},
		{"cvc", "999", "[REDACTED]"},
		{"pin", "1234", "[REDACTED]"},
		{"payment_credential", "secret-payment-payload", "[REDACTED]"},
		{"raw_jwt_value", "eyJhbGciOiJIUzI1NiIsInR5cCI6IkpXVCJ9.eyJzdWIiOiIxMjM0NTY3ODkwIn0.doNotLeakThisSignature", "[REDACTED]"},
		{"bearer_auth_header", "Bearer eyJhbGciOi...", "[REDACTED]"},
		{"email", "user@test.com", "user@test.com"},
		{"username", "john_doe", "john_doe"},
		{"request_id", "req-123", "req-123"},
	}

	for _, tc := range testCases {
		attr := RedactAttr(nil, slog.String(tc.key, tc.val))
		if attr.Value.String() != tc.expected {
			t.Errorf("Key '%s' with value '%s': expected '%s', got '%s'", tc.key, tc.val, tc.expected, attr.Value.String())
		}
	}
}

func TestLoggerLocationMark(t *testing.T) {
	t.Run("default configuration includes location mark", func(t *testing.T) {
		var buf bytes.Buffer
		l := New(Config{
			ServiceName: "test-service",
			Output:      &buf,
		})

		l.Info("testing default location mark")

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse log JSON: %v, raw: %s", err, buf.String())
		}

		source, ok := logEntry["source"].(map[string]any)
		if !ok || source == nil {
			t.Fatalf("Expected 'source' location mark to be present, got: %v", logEntry)
		}
		file, ok := source["file"].(string)
		if !ok || file == "" || !strings.Contains(file, ":") {
			t.Errorf("Expected 'file' to contain file:line location, got: %v", source["file"])
		}
		loc, ok := source["location"].(string)
		if !ok || loc == "" || !strings.Contains(loc, ":") {
			t.Errorf("Expected 'location' to contain file:line location, got: %v", source["location"])
		}
		if line, ok := source["line"].(float64); !ok || line == 0 {
			t.Errorf("Expected non-zero 'line' in location mark, got: %v", source["line"])
		}
		if function, ok := source["function"].(string); !ok || function == "" {
			t.Errorf("Expected 'function' in location mark, got: %v", source["function"])
		}
	})

	t.Run("DisableSource disables location mark", func(t *testing.T) {
		var buf bytes.Buffer
		l := New(Config{
			ServiceName:   "test-service",
			DisableSource: true,
			Output:        &buf,
		})

		l.Info("testing disabled location mark")

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse log JSON: %v", err)
		}

		if _, exists := logEntry["source"]; exists {
			t.Errorf("Expected 'source' to be absent when DisableSource is true, got: %v", logEntry["source"])
		}
	})

	t.Run("LOG_ADD_SOURCE=false disables location mark", func(t *testing.T) {
		t.Setenv("LOG_ADD_SOURCE", "false")
		var buf bytes.Buffer
		l := New(Config{
			ServiceName: "test-service",
			Output:      &buf,
		})

		l.Info("testing LOG_ADD_SOURCE=false")

		var logEntry map[string]any
		if err := json.Unmarshal(buf.Bytes(), &logEntry); err != nil {
			t.Fatalf("Failed to parse log JSON: %v", err)
		}

		if _, exists := logEntry["source"]; exists {
			t.Errorf("Expected 'source' to be absent when LOG_ADD_SOURCE=false, got: %v", logEntry["source"])
		}
	})

	t.Run("text format includes location mark", func(t *testing.T) {
		var buf bytes.Buffer
		l := New(Config{
			ServiceName: "test-service",
			Format:      "text",
			Output:      &buf,
		})

		l.Info("testing text format location mark")

		raw := buf.String()
		if !bytes.Contains(buf.Bytes(), []byte("source=")) {
			t.Errorf("Expected 'source=' in text log output, got: %s", raw)
		}
	})
}

