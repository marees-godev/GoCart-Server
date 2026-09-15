package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/tracing"
)

func TestMiddlewareStack(t *testing.T) {
	var buf bytes.Buffer
	l := logger.New(logger.Config{
		ServiceName: "test-auth",
		Environment: "test",
		Level:       "DEBUG",
		Format:      "json",
		Output:      &buf,
	})

	_, _ = tracing.Init(tracing.Config{
		ServiceName: "test-auth",
		Environment: "test",
		Enabled:     false,
		Exporter:    "noop",
	})

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		l.InfoContext(r.Context(), "Handler executed", "auth_token", "secret-12345")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	})

	stack := Recovery(
		RequestID(
			Tracing("test-auth")(
				Metrics("test-auth")(
					Logger(handler),
				),
			),
		),
	)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile", nil)
	req.Header.Set("X-Request-ID", "custom-req-id-777")
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-Request-ID") != "custom-req-id-777" {
		t.Errorf("Expected response X-Request-ID 'custom-req-id-777', got '%s'", rec.Header().Get("X-Request-ID"))
	}

	rawLog := buf.String()
	if rawLog == "" {
		t.Fatal("Expected logs to be written to buffer")
	}

	var logEntry map[string]any
	if err := json.Unmarshal([]byte(rawLog[:bytes.IndexByte(buf.Bytes(), '\n')]), &logEntry); err != nil {
		t.Fatalf("Failed to parse first log line as JSON: %v", err)
	}

	if logEntry["request_id"] != "custom-req-id-777" {
		t.Errorf("Expected request_id 'custom-req-id-777' in log entry, got '%v'", logEntry["request_id"])
	}
	if logEntry["auth_token"] != "[REDACTED]" {
		t.Errorf("Expected auth_token to be '[REDACTED]', got '%v'", logEntry["auth_token"])
	}
}

func TestPanicRecovery(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection exploded")
	})

	stack := Recovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crash", nil)
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status 500 on panic recovery, got %d", rec.Code)
	}

	var res map[string]string
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if res["error"] != "Internal Server Error" {
		t.Errorf("Expected error 'Internal Server Error', got '%s'", res["error"])
	}
}
