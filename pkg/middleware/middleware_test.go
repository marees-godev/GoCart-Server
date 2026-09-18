package middleware

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/pkg/response"
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

	var ctxReqID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ctxReqID = GetRequestID(r.Context())
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

	req := httptest.NewRequest(http.MethodGet, "/api/v1/profile?token=secret123", nil)
	req.Header.Set("X-Request-ID", "custom-req-id-777")
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("X-Request-ID") != "custom-req-id-777" {
		t.Errorf("Expected response X-Request-ID 'custom-req-id-777', got '%s'", rec.Header().Get("X-Request-ID"))
	}

	if ctxReqID != "custom-req-id-777" {
		t.Errorf("Expected GetRequestID context value 'custom-req-id-777', got '%s'", ctxReqID)
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

func TestRequestIDGenerationWhenMissing(t *testing.T) {
	var capturedID string
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedID = GetRequestID(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	stack := RequestID(handler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/resource", nil)
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if capturedID == "" {
		t.Error("Expected generated request ID in context, got empty string")
	}

	respHeader := rec.Header().Get("X-Request-ID")
	if respHeader == "" {
		t.Error("Expected X-Request-ID response header to be set")
	}

	if respHeader != capturedID {
		t.Errorf("Expected header ID %s to match context ID %s", respHeader, capturedID)
	}
}

func TestPanicRecovery(t *testing.T) {
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("database connection exploded with secret password=12345")
	})

	stack := Recovery(panicHandler)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/crash", nil)
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("Expected status 500 on panic recovery, got %d", rec.Code)
	}

	var res response.ErrorResponse
	if err := json.NewDecoder(rec.Body).Decode(&res); err != nil {
		t.Fatalf("Failed to decode JSON response: %v", err)
	}

	if res.Error.Code != "INTERNAL_SERVER_ERROR" {
		t.Errorf("Expected error code 'INTERNAL_SERVER_ERROR', got '%s'", res.Error.Code)
	}
	if res.Error.Message != "An internal server error occurred" {
		t.Errorf("Expected sanitized message 'An internal server error occurred', got '%s'", res.Error.Message)
	}
}
