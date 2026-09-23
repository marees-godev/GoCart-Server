package tests

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
)

func setupTestAppWithCORS(cfg *config.Config) *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	app.Use(cors.New(cors.Config{
		AllowOrigins:     strings.Join(cfg.CORS.AllowedOrigins, ", "),
		AllowMethods:     strings.Join(cfg.CORS.AllowedMethods, ", "),
		AllowHeaders:     strings.Join(cfg.CORS.AllowedHeaders, ", "),
		ExposeHeaders:    strings.Join(cfg.CORS.ExposedHeaders, ", "),
		AllowCredentials: cfg.CORS.AllowCredentials,
		MaxAge:           cfg.CORS.MaxAge,
	}))

	app.Post("/query", func(c *fiber.Ctx) error {
		return c.SendString("ok")
	})

	return app
}

func TestGatewayCORS_PreflightOptions(t *testing.T) {
	cfg := config.LoadEnv()
	app := setupTestAppWithCORS(cfg)

	req := httptest.NewRequest(http.MethodOptions, "/query", nil)
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Access-Control-Request-Method", "POST")
	req.Header.Set("Access-Control-Request-Headers", "Content-Type, Authorization")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 204 or 200 for OPTIONS preflight, got %d", resp.StatusCode)
	}

	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" && allowOrigin != "http://localhost:3000" {
		t.Errorf("Unexpected Access-Control-Allow-Origin header: %s", allowOrigin)
	}
}

func TestGatewayCORS_ActualRequest(t *testing.T) {
	cfg := config.LoadEnv()
	app := setupTestAppWithCORS(cfg)

	body := []byte(`{"query":"{ health }"}`)
	req := httptest.NewRequest(http.MethodPost, "/query", bytes.NewReader(body))
	req.Header.Set("Origin", "http://localhost:3000")
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to execute request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status 200, got %d", resp.StatusCode)
	}

	allowOrigin := resp.Header.Get("Access-Control-Allow-Origin")
	if allowOrigin != "*" && allowOrigin != "http://localhost:3000" {
		t.Errorf("Unexpected Access-Control-Allow-Origin header: %s", allowOrigin)
	}
}
