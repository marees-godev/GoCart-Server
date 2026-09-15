package tracing

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestTracingInitAndContextHelpers(t *testing.T) {
	cfg := Config{
		ServiceName: "test-service",
		Environment: "test",
		Version:     "v1.0.0",
		Enabled:     false,
		Exporter:    "noop",
	}

	tp, err := Init(cfg)
	if err != nil {
		t.Fatalf("Failed to initialize tracing: %v", err)
	}
	if tp != nil {
		defer func() { _ = tp.Shutdown(context.Background()) }()
	}

	ctx := context.Background()
	ctx, span := StartSpan(ctx, "test-operation")
	defer span.End()

	// HTTP context propagation
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	InjectHTTPHeaders(ctx, req)

	extractedCtx := ExtractHTTPHeaders(req)
	if extractedCtx == nil {
		t.Fatal("Expected extracted context to not be nil")
	}

	// Message carrier propagation
	msgHeaders := make(map[string]string)
	InjectMessageContext(ctx, msgHeaders)

	msgCtx := ExtractMessageContext(context.Background(), msgHeaders)
	if msgCtx == nil {
		t.Fatal("Expected message extracted context to not be nil")
	}

	// Byte message carrier propagation
	byteHeaders := make(map[string][]byte)
	InjectByteMessageContext(ctx, byteHeaders)

	byteCtx := ExtractByteMessageContext(context.Background(), byteHeaders)
	if byteCtx == nil {
		t.Fatal("Expected byte message extracted context to not be nil")
	}
}
