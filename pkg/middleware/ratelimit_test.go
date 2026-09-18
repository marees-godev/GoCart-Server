package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRateLimiter(t *testing.T) {
	limiter := NewRateLimiter(RateLimiterConfig{
		MaxRequests: 3,
		Window:      100 * time.Millisecond,
		KeyFunc: func(r *http.Request) string {
			return r.Header.Get("X-Client-ID")
		},
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	}))

	// 1. Make 3 allowed requests
	for i := 1; i <= 3; i++ {
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Client-ID", "client-1")
		rec := httptest.NewRecorder()

		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: expected status 200, got %d", i, rec.Code)
		}
	}

	// 2. 4th request must be rate limited (429)
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.Header.Set("X-Client-ID", "client-1")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Request 4: expected status 429, got %d", rec.Code)
	}

	if rec.Header().Get("Retry-After") == "" {
		t.Errorf("Expected Retry-After header to be set on 429 response")
	}

	// 3. Different client key should still be allowed
	req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req2.Header.Set("X-Client-ID", "client-2")
	rec2 := httptest.NewRecorder()

	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("Different client: expected status 200, got %d", rec2.Code)
	}

	// 4. After window expires, original client is allowed again
	time.Sleep(120 * time.Millisecond)

	req3 := httptest.NewRequest(http.MethodGet, "/test", nil)
	req3.Header.Set("X-Client-ID", "client-1")
	rec3 := httptest.NewRecorder()

	handler.ServeHTTP(rec3, req3)

	if rec3.Code != http.StatusOK {
		t.Errorf("After window expiry: expected status 200, got %d", rec3.Code)
	}
}

func TestRateLimiter_BypassHealthMetrics(t *testing.T) {
	limiter := NewRateLimiter(RateLimiterConfig{
		MaxRequests: 1,
		Window:      time.Minute,
	})

	handler := limiter.Middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	for _, path := range []string{"/health", "/ready", "/metrics"} {
		for i := 0; i < 5; i++ {
			req := httptest.NewRequest(http.MethodGet, path, nil)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)
			if rec.Code != http.StatusOK {
				t.Errorf("Path %s request %d: expected 200 bypass, got %d", path, i, rec.Code)
			}
		}
	}
}
