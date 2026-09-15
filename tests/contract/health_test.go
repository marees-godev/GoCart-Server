package contract_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	"github.com/marees-godev/GoCart-Server/pkg/health"
	"github.com/marees-godev/GoCart-Server/pkg/metrics"
)

type mockPinger struct {
	err error
}

func (m mockPinger) Ping(ctx context.Context) error {
	return m.err
}

var services = []string{
	"auth-service",
	"cart-service",
	"category-service",
	"delivery-service",
	"inventory-service",
	"merchant-service",
	"notification-service",
	"order-service",
	"payment-service",
	"product-service",
	"rating-service",
	"return-service",
	"store-service",
	"user-service",
}

func setupTestApp(serviceName string, pinger health.Pinger) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	healthHandler := health.NewHandler(serviceName, health.FromPinger(pinger))
	healthHandler.Register(app)
	app.Get("/metrics", adaptor.HTTPHandler(metrics.Handler()))

	return app
}

func TestServicesHealthEndpoint(t *testing.T) {
	for _, service := range services {
		t.Run(service+"/health", func(t *testing.T) {
			app := setupTestApp(service, mockPinger{err: nil})

			req := httptest.NewRequest(http.MethodGet, "/health", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("[%s] unexpected error testing /health: %v", service, err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("[%s] expected /health status 200, got %d", service, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			expectedBody := `{"status":"ok","service":"` + service + `"}`
			if string(body) != expectedBody {
				t.Errorf("[%s] expected body %q, got %q", service, expectedBody, string(body))
			}
		})
	}
}

func TestServicesReadyEndpoint_Healthy(t *testing.T) {
	for _, service := range services {
		t.Run(service+"/ready_healthy", func(t *testing.T) {
			app := setupTestApp(service, mockPinger{err: nil})

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("[%s] unexpected error testing /ready: %v", service, err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("[%s] expected /ready status 200, got %d", service, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			expectedBody := `{"status":"ok","service":"` + service + `"}`
			if string(body) != expectedBody {
				t.Errorf("[%s] expected body %q, got %q", service, expectedBody, string(body))
			}
		})
	}
}

func TestServicesReadyEndpoint_Unhealthy(t *testing.T) {
	for _, service := range services {
		t.Run(service+"/ready_unhealthy", func(t *testing.T) {
			app := setupTestApp(service, mockPinger{err: errors.New("database connection down")})

			req := httptest.NewRequest(http.MethodGet, "/ready", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("[%s] unexpected error testing /ready: %v", service, err)
			}

			if resp.StatusCode != http.StatusServiceUnavailable {
				t.Errorf("[%s] expected /ready status 503, got %d", service, resp.StatusCode)
			}

			body, _ := io.ReadAll(resp.Body)
			expectedBody := `{"status":"unavailable","service":"` + service + `"}`
			if string(body) != expectedBody {
				t.Errorf("[%s] expected body %q, got %q", service, expectedBody, string(body))
			}
		})
	}
}

func TestServicesMetricsEndpoint(t *testing.T) {
	for _, service := range services {
		t.Run(service+"/metrics", func(t *testing.T) {
			app := setupTestApp(service, mockPinger{err: nil})

			req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
			resp, err := app.Test(req)
			if err != nil {
				t.Fatalf("[%s] unexpected error testing /metrics: %v", service, err)
			}

			if resp.StatusCode != http.StatusOK {
				t.Errorf("[%s] expected /metrics status 200, got %d", service, resp.StatusCode)
			}
		})
	}
}
