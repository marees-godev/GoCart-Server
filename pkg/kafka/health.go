package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"time"
)

// HealthResult represents the status and metrics of the Kafka broker connection.
type HealthResult struct {
	Status    string   `json:"status"` // "UP" or "DOWN"
	Brokers   []string `json:"brokers"`
	LatencyMs int64    `json:"latency_ms"`
	Error     string   `json:"error,omitempty"`
}

// CheckHealth checks connectivity to configured Kafka brokers and measures response latency.
func CheckHealth(ctx context.Context, cfg Config) HealthResult {
	if len(cfg.Brokers) == 0 {
		return HealthResult{
			Status:  "DOWN",
			Brokers: cfg.Brokers,
			Error:   "no kafka brokers configured",
		}
	}

	start := time.Now()
	dialer := &net.Dialer{
		Timeout: cfg.ConnectTimeout,
	}

	// Dial the primary broker
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Brokers[0])
	latency := time.Since(start).Milliseconds()

	if err != nil {
		return HealthResult{
			Status:    "DOWN",
			Brokers:   cfg.Brokers,
			LatencyMs: latency,
			Error:     fmt.Sprintf("failed to connect to broker %s: %v", cfg.Brokers[0], err),
		}
	}
	_ = conn.Close()

	return HealthResult{
		Status:    "UP",
		Brokers:   cfg.Brokers,
		LatencyMs: latency,
	}
}

// HealthHandler returns an http.HandlerFunc that exposes Kafka health status over HTTP (e.g. GET /health/kafka).
func HealthHandler(cfg Config) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		result := CheckHealth(r.Context(), cfg)

		w.Header().Set("Content-Type", "application/json")
		if result.Status == "UP" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		_ = json.NewEncoder(w).Encode(result)
	}
}
