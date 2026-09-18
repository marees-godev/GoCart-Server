package kafka_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/kafka"
)

func TestCheckHealthLiveOrMock(t *testing.T) {
	cfg := kafka.DefaultConfig()
	cfg.Brokers = []string{"localhost:9092"}
	cfg.ConnectTimeout = 2 * time.Second

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	result := kafka.CheckHealth(ctx, cfg)
	if result.Status != "UP" && result.Status != "DOWN" {
		t.Errorf("expected status UP or DOWN, got %s", result.Status)
	}

	t.Logf("Kafka Health Check Status: %s, Latency: %d ms, Error: %s", result.Status, result.LatencyMs, result.Error)
}

func TestHealthHandler(t *testing.T) {
	cfg := kafka.DefaultConfig()
	cfg.Brokers = []string{"localhost:9092"}

	handler := kafka.HealthHandler(cfg)

	req := httptest.NewRequest(http.MethodGet, "/health/kafka", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if rec.Code != http.StatusOK && rec.Code != http.StatusServiceUnavailable {
		t.Errorf("expected status 200 or 533, got %d", rec.Code)
	}
}
