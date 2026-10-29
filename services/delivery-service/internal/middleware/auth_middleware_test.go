package middleware

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
)

func TestDeliveryPartnerAuthMiddleware(t *testing.T) {
	jwtSecret := "test-secret-key-12345"

	app := fiber.New()
	app.Get("/test-partner", DeliveryPartnerAuthMiddleware(jwtSecret), func(c *fiber.Ctx) error {
		pID := GetPartnerID(c)
		return c.Status(fiber.StatusOK).JSON(fiber.Map{
			"partner_id": pID,
		})
	})

	t.Run("Rejects request when JWT secret is empty/unconfigured", func(t *testing.T) {
		appEmpty := fiber.New()
		appEmpty.Get("/test-partner", DeliveryPartnerAuthMiddleware(""), func(c *fiber.Ctx) error {
			return c.Status(fiber.StatusOK).JSON(fiber.Map{"status": "ok"})
		})

		req := httptest.NewRequest("GET", "/test-partner", nil)
		req.Header.Set("Authorization", "Bearer some-token")
		resp, err := appEmpty.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusInternalServerError {
			t.Errorf("expected 500 when secret is unconfigured, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var res map[string]interface{}
		_ = json.Unmarshal(body, &res)
		errMap, ok := res["error"].(map[string]interface{})
		if !ok || errMap["message"] != "authentication cannot be configured: JWT secret is missing" {
			t.Errorf("expected error message 'authentication cannot be configured: JWT secret is missing', got: %v", res)
		}
	})

	t.Run("Missing Authorization Header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/test-partner", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Invalid Token Signature", func(t *testing.T) {
		tok, _ := auth.GenerateToken(auth.UserContext{
			UserID:    "user-1",
			Role:      "DELIVERY_PARTNER",
			PartnerID: "partner-1",
		}, "wrong-secret", 15*time.Minute)

		req := httptest.NewRequest("GET", "/test-partner", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("Forbidden for USER or MERCHANT roles", func(t *testing.T) {
		tok, _ := auth.GenerateToken(auth.UserContext{
			UserID: "user-1",
			Role:   "USER",
		}, jwtSecret, 15*time.Minute)

		req := httptest.NewRequest("GET", "/test-partner", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("Valid DELIVERY_PARTNER Token", func(t *testing.T) {
		tok, _ := auth.GenerateToken(auth.UserContext{
			UserID:    "user-1",
			Role:      "DELIVERY_PARTNER",
			PartnerID: "partner-999",
		}, jwtSecret, 15*time.Minute)

		req := httptest.NewRequest("GET", "/test-partner", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("Valid ADMIN Token", func(t *testing.T) {
		tok, _ := auth.GenerateToken(auth.UserContext{
			UserID: "admin-1",
			Role:   "ADMIN",
		}, jwtSecret, 15*time.Minute)

		req := httptest.NewRequest("GET", "/test-partner", nil)
		req.Header.Set("Authorization", "Bearer "+tok)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.StatusCode != fiber.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
