package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

func TestPartnerWarehouseManagement(t *testing.T) {
	jwtSecret := "delivery-service-test-jwt-secret-12345"
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	app := fiber.New()
	deliveryHandler := handler.NewDeliveryHandler(svc, jwtSecret)
	deliveryHandler.RegisterRoutes(app)

	ctx := context.Background()

	// 1. Seed warehouses
	warehouse1 := &model.Warehouse{
		ID:         uuid.New(),
		Name:       "West Hub Seattle",
		Code:       "WH-SEA-01",
		Street:     "100 First Ave",
		City:       "Seattle",
		State:      "WA",
		PostalCode: "98101",
		IsActive:   true,
	}
	warehouse2 := &model.Warehouse{
		ID:         uuid.New(),
		Name:       "South Hub Austin",
		Code:       "WH-ATX-01",
		Street:     "200 Congress Ave",
		City:       "Austin",
		State:      "TX",
		PostalCode: "73301",
		IsActive:   true,
	}
	_ = repo.CreateWarehouse(ctx, warehouse1)
	_ = repo.CreateWarehouse(ctx, warehouse2)

	// 2. Test GET /api/warehouses/available
	t.Run("GET /api/warehouses/available", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/warehouses/available", nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		var list []*dto.WarehouseResponse
		if err := json.Unmarshal(bodyBytes, &list); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if len(list) != 2 {
			t.Errorf("expected 2 warehouses, got %d", len(list))
		}
	})

	// 3. Test PartnerRegistered event consumer
	partnerID := uuid.New()
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

	// 4. Generate tokens
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

	// 5. Test Access Control on POST /api/partners/warehouses
	t.Run("Access Control - Reject without token or wrong role", func(t *testing.T) {
		// Without token
		reqNoAuth := httptest.NewRequest(http.MethodPost, "/api/partners/warehouses", nil)
		respNoAuth, _ := app.Test(reqNoAuth)
		if respNoAuth.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401 for missing token, got %d", respNoAuth.StatusCode)
		}

		// With USER token
		reqUser := httptest.NewRequest(http.MethodPost, "/api/partners/warehouses", nil)
		reqUser.Header.Set("Authorization", "Bearer "+userToken)
		respUser, _ := app.Test(reqUser)
		if respUser.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403 for USER role, got %d", respUser.StatusCode)
		}
	})

	// 6. Test POST /api/partners/warehouses
	t.Run("POST /api/partners/warehouses links warehouse to partner", func(t *testing.T) {
		body, _ := json.Marshal(dto.AssignWarehousesRequest{
			WarehouseIDs: []string{warehouse1.ID.String()},
		})
		req := httptest.NewRequest(http.MethodPost, "/api/partners/warehouses", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+partnerToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		// Verify GET /api/partners/me/warehouses returns the linked warehouse
		getReq := httptest.NewRequest(http.MethodGet, "/api/partners/me/warehouses", nil)
		getReq.Header.Set("Authorization", "Bearer "+partnerToken)
		getResp, err := app.Test(getReq)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if getResp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", getResp.StatusCode)
		}

		getRespBytes, _ := io.ReadAll(getResp.Body)
		var partnerWarehouses []*dto.WarehouseResponse
		if err := json.Unmarshal(getRespBytes, &partnerWarehouses); err != nil {
			t.Fatalf("failed to parse partner warehouses response: %v", err)
		}
		if len(partnerWarehouses) != 1 {
			t.Fatalf("expected 1 assigned warehouse, got %d", len(partnerWarehouses))
		}
		if partnerWarehouses[0].ID != warehouse1.ID.String() {
			t.Errorf("expected warehouse ID %s, got %s", warehouse1.ID, partnerWarehouses[0].ID)
		}
	})

	// 7. Test GET /api/partners/me retrieves delivery partner profile once registered and logged in
	t.Run("GET /api/partners/me retrieves profile", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodGet, "/api/partners/me", nil)
		req.Header.Set("Authorization", "Bearer "+partnerToken)

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		var prof dto.PartnerProfileResponse
		if err := json.Unmarshal(bodyBytes, &prof); err != nil {
			t.Fatalf("failed to parse profile response: %v", err)
		}
		if prof.PartnerID != partnerID.String() {
			t.Errorf("expected partnerID %s, got %s", partnerID, prof.PartnerID)
		}
		if prof.CompanyName != "Express Delivery LLC" {
			t.Errorf("expected company name 'Express Delivery LLC', got %s", prof.CompanyName)
		}
	})

	// 8. Test PUT /api/partners/me updates delivery partner profile
	t.Run("PUT /api/partners/me updates partner details", func(t *testing.T) {
		newCompName := "Express Global Logistics"
		newContact := "+1-206-555-9999"
		body, _ := json.Marshal(dto.UpdatePartnerProfileRequest{
			CompanyName:   &newCompName,
			ContactNumber: &newContact,
		})
		req := httptest.NewRequest(http.MethodPut, "/api/partners/me", bytes.NewReader(body))
		req.Header.Set("Authorization", "Bearer "+partnerToken)
		req.Header.Set("Content-Type", "application/json")

		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		bodyBytes, _ := io.ReadAll(resp.Body)
		var prof dto.PartnerProfileResponse
		if err := json.Unmarshal(bodyBytes, &prof); err != nil {
			t.Fatalf("failed to parse profile response: %v", err)
		}
		if prof.CompanyName != "Express Global Logistics" {
			t.Errorf("expected updated company name, got %s", prof.CompanyName)
		}
		if prof.ContactNumber != "+1-206-555-9999" {
			t.Errorf("expected updated contact number, got %s", prof.ContactNumber)
		}
	})
}

