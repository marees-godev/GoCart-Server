package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

func TestPartnerProfileManagement(t *testing.T) {
	jwtSecret := "delivery-service-test-jwt-secret-12345"
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	app := fiber.New()
	deliveryHandler := handler.NewDeliveryHandler(svc, jwtSecret)
	deliveryHandler.RegisterRoutes(app)

	ctx := context.Background()
	partnerID := uuid.New()

	// 1. Test PartnerRegistered event consumer
	t.Run("Consume PartnerRegistered event", func(t *testing.T) {
		regEvt := events.PartnerRegisteredEvent{
			PartnerID:     partnerID.String(),
			CompanyName:   "Express Delivery LLC",
			Email:         "express@partner.com",
			ContactNumber: "+1-206-555-0199",
		}
		env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePartnerRegistered, "auth-service", partnerID.String(), regEvt)
		if err != nil {
			t.Fatalf("failed to create envelope: %v", err)
		}
		if err := eventConsumer.HandleEvent(ctx, env); err != nil {
			t.Fatalf("expected PartnerRegistered event to be handled, got: %v", err)
		}

		profile, err := repo.GetPartnerProfile(ctx, partnerID)
		if err != nil {
			t.Fatalf("expected partner profile in repo, got err: %v", err)
		}
		if profile.CompanyName != "Express Delivery LLC" {
			t.Errorf("expected company name 'Express Delivery LLC', got %s", profile.CompanyName)
		}
	})

	// 2. Generate tokens
	partnerToken, _ := auth.GenerateToken(auth.UserContext{
		UserID:    partnerID.String(),
		Role:      "DELIVERY_PARTNER",
		PartnerID: partnerID.String(),
		Email:     "express@partner.com",
	}, jwtSecret, 15*time.Minute)

	userToken, _ := auth.GenerateToken(auth.UserContext{
		UserID:     uuid.NewString(),
		Role:       "USER",
		CustomerID: uuid.NewString(),
		Email:      "user@example.com",
	}, jwtSecret, 15*time.Minute)

	// 3. Access Control
	t.Run("Access Control - Reject without token or wrong role", func(t *testing.T) {
		reqNoAuth := httptest.NewRequest(http.MethodGet, "/api/partners/me", nil)
		respNoAuth, _ := app.Test(reqNoAuth)
		if respNoAuth.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing token, got %d", respNoAuth.StatusCode)
		}

		reqUser := httptest.NewRequest(http.MethodGet, "/api/partners/me", nil)
		reqUser.Header.Set("Authorization", "Bearer "+userToken)
		respUser, _ := app.Test(reqUser)
		if respUser.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 for USER role, got %d", respUser.StatusCode)
		}
	})

	// 4. Test Profile Endpoints
	t.Run("GET /api/partners/me and PUT /api/partners/me", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/partners/me", nil)
		req.Header.Set("Authorization", "Bearer "+partnerToken)
		resp, _ := app.Test(req)
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		newComp := "Express Prime Logistics"
		body, _ := json.Marshal(dto.UpdatePartnerProfileRequest{CompanyName: &newComp})
		putReq := httptest.NewRequest(http.MethodPut, "/api/partners/me", bytes.NewReader(body))
		putReq.Header.Set("Authorization", "Bearer "+partnerToken)
		putReq.Header.Set("Content-Type", "application/json")
		putResp, _ := app.Test(putReq)
		if putResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", putResp.StatusCode)
		}

		updated, err := repo.GetPartnerProfile(ctx, partnerID)
		if err != nil {
			t.Fatalf("failed to fetch updated profile: %v", err)
		}
		if updated.CompanyName != newComp {
			t.Errorf("expected %s, got %s", newComp, updated.CompanyName)
		}
	})
}
