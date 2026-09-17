package tests

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/adaptor"
	gwGraphQL "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql"
	gwResolver "github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/resolvers"
	"github.com/marees-godev/GoCart-Server/pkg/health"
)

func setupTestApp() *fiber.App {
	app := fiber.New(fiber.Config{
		DisableStartupMessage: true,
	})

	healthHandler := health.NewHandler("api-gateway")
	healthHandler.Register(app)

	gqlResolver := gwResolver.NewResolver(nil, "1.0.0")
	gqlServer := gwGraphQL.NewServer(gqlResolver)

	app.All("/graphql", adaptor.HTTPHandler(gqlServer))
	app.Get("/playground", adaptor.HTTPHandler(gwGraphQL.PlaygroundHandler("Playground", "/graphql")))

	return app
}

func TestHealthEndpoint(t *testing.T) {
	app := setupTestApp()

	req := httptest.NewRequest(http.MethodGet, "/health", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var healthResp map[string]string
	_ = json.Unmarshal(body, &healthResp)

	if healthResp["status"] != "ok" || healthResp["service"] != "api-gateway" {
		t.Errorf("unexpected health response: %s", string(body))
	}
}

func TestReadyEndpoint(t *testing.T) {
	app := setupTestApp()

	req := httptest.NewRequest(http.MethodGet, "/ready", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestGraphQLEndpoint(t *testing.T) {
	app := setupTestApp()

	queryBody := []byte(`{"query":"{ health version }"}`)
	req := httptest.NewRequest(http.MethodPost, "/graphql", bytes.NewReader(queryBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var gqlResp map[string]any
	_ = json.Unmarshal(body, &gqlResp)

	data, ok := gqlResp["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object in GraphQL response, got %s", string(body))
	}

	if data["health"] != "OK" {
		t.Errorf("expected data.health == 'OK', got %v", data["health"])
	}
}

func TestPlaygroundEndpoint(t *testing.T) {
	app := setupTestApp()

	req := httptest.NewRequest(http.MethodGet, "/playground", nil)
	resp, err := app.Test(req, -1)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}
