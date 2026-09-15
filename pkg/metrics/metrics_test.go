package metrics

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMetricsRecordingAndHandler(t *testing.T) {
	RecordHTTPRequest("auth-service", "POST", "/api/v1/login", 200, 45*time.Millisecond)
	IncHTTPInFlight("auth-service")
	DecHTTPInFlight("auth-service")
	RecordMessageProcessed("order-service", "order.created", "success", 120*time.Millisecond)
	RecordMessagePublished("order-service", "order.created", "success")
	RecordDBQuery("auth-service", "SELECT", "users", 12*time.Millisecond)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	rec := httptest.NewRecorder()

	h := Handler()
	h.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("Expected status 200 from /metrics handler, got %d", rec.Code)
	}

	body := rec.Body.String()
	if !strings.Contains(body, "gocart_http_requests_total") {
		t.Errorf("Expected body to contain 'gocart_http_requests_total', got:\n%s", body)
	}
	if !strings.Contains(body, "gocart_message_processed_total") {
		t.Errorf("Expected body to contain 'gocart_message_processed_total', got:\n%s", body)
	}
}
