package health_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/health"
)

type mockPinger struct {
	err error
}

func (m mockPinger) Ping(ctx context.Context) error {
	return m.err
}

func TestFiberHealthEndpoint(t *testing.T) {
	app := fiber.New()
	handler := health.NewHandler("test-service")
	handler.Register(app)

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expectedBody := `{"status":"ok","service":"test-service"}`
	if string(body) != expectedBody {
		t.Fatalf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestFiberReadyEndpoint_Success(t *testing.T) {
	app := fiber.New()
	pinger := mockPinger{err: nil}
	handler := health.NewHandler("test-service", health.FromPinger(pinger))
	handler.Register(app)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expectedBody := `{"status":"ok","service":"test-service"}`
	if string(body) != expectedBody {
		t.Fatalf("expected body %q, got %q", expectedBody, string(body))
	}
}

func TestFiberReadyEndpoint_Failure(t *testing.T) {
	app := fiber.New()
	pinger := mockPinger{err: errors.New("connection failed")}
	handler := health.NewHandler("test-service", health.FromPinger(pinger))
	handler.Register(app)

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("expected status code %d, got %d", http.StatusServiceUnavailable, resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	expectedBody := `{"status":"unavailable","service":"test-service"}`
	if string(body) != expectedBody {
		t.Fatalf("expected body %q, got %q", expectedBody, string(body))
	}
}
