package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCORSMiddleware_PreflightWildcard(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	stack := CORS()(handler)

	req := httptest.NewRequest(http.MethodOptions, "/graphql", nil)
	req.Header.Set("Origin", "http://example.com")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusNoContent {
		t.Fatalf("Expected status 204 No Content for OPTIONS preflight, got %d", rec.Code)
	}

	if rec.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin '*', got '%s'", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	if rec.Header().Get("Access-Control-Allow-Headers") != "Content-Type, Authorization" {
		t.Errorf("Expected requested headers in Access-Control-Allow-Headers, got '%s'", rec.Header().Get("Access-Control-Allow-Headers"))
	}
}

func TestCORSMiddleware_SpecificOriginsAndCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})

	cfg := CORSConfig{
		AllowedOrigins:   []string{"http://localhost:3000", "https://app.gocart.com"},
		AllowedMethods:   []string{"GET", "POST"},
		AllowedHeaders:   []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
	}

	stack := CORS(cfg)(handler)

	// Test allowed origin request
	req := httptest.NewRequest(http.MethodPost, "/query", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	rec := httptest.NewRecorder()

	stack.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200, got %d", rec.Code)
	}

	if rec.Header().Get("Access-Control-Allow-Origin") != "http://localhost:3000" {
		t.Errorf("Expected Access-Control-Allow-Origin 'http://localhost:3000', got '%s'", rec.Header().Get("Access-Control-Allow-Origin"))
	}

	if rec.Header().Get("Access-Control-Allow-Credentials") != "true" {
		t.Errorf("Expected Access-Control-Allow-Credentials 'true', got '%s'", rec.Header().Get("Access-Control-Allow-Credentials"))
	}

	// Test disallowed origin request
	reqDisallowed := httptest.NewRequest(http.MethodPost, "/query", nil)
	reqDisallowed.Header.Set("Origin", "http://malicious.com")
	recDisallowed := httptest.NewRecorder()

	stack.ServeHTTP(recDisallowed, reqDisallowed)

	if recDisallowed.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("Expected empty Access-Control-Allow-Origin for disallowed origin, got '%s'", recDisallowed.Header().Get("Access-Control-Allow-Origin"))
	}
}
