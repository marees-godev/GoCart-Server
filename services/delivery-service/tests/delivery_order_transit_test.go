package tests

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type mockOrderClient struct {
	orders map[string]*client.OrderDetails
}

func newMockOrderClient() *mockOrderClient {
	return &mockOrderClient{
		orders: make(map[string]*client.OrderDetails),
	}
}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID string) (*client.OrderDetails, error) {
	if ord, ok := m.orders[orderID]; ok {
		return ord, nil
	}
	return nil, appErrors.NotFound("order not found in order_service")
}

func (m *mockOrderClient) AddOrder(ord *client.OrderDetails) {
	m.orders[ord.ID] = ord
}

func TestDeliveryOrderService_TransitLogic_And_DeliveryFlow(t *testing.T) {
	ctx := context.Background()

	repo := newMemoryDeliveryRepo()
	orderClient := newMockOrderClient()
	svc := service.NewDeliveryService(repo, orderClient)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	app := fiber.New()
	deliveryHandler := handler.NewDeliveryHandler(svc)
	deliveryHandler.RegisterRoutes(app)

	partnerID := uuid.New()

	// 1. Seed partner warehouses:
	// - Austin, TX (active)
	// - Dallas, TX (inactive - should not be selected)
	austinWarehouse := &model.Warehouse{
		ID:                uuid.New(),
		DeliveryPartnerID: partnerID,
		Name:              "Austin Fulfillment Hub",
		Code:              "WH-ATX-01",
		City:              "Austin",
		State:             "TX",
		IsActive:          true,
	}
	_ = repo.CreateWarehouse(ctx, austinWarehouse)

	dallasInactiveWH := &model.Warehouse{
		ID:                uuid.New(),
		DeliveryPartnerID: partnerID,
		Name:              "Dallas Inactive Hub",
		Code:              "WH-DFW-01",
		City:              "Dallas",
		State:             "TX",
		IsActive:          false,
	}
	_ = repo.CreateWarehouse(ctx, dallasInactiveWH)

	// Case 1: Same-city delivery: 1–2 days (target: 2 days)
	// Destination: Austin, TX. Origin warehouse: Austin, TX
	t.Run("Same-city delivery 1-2 days", func(t *testing.T) {
		orderID := uuid.NewString()
		orderClient.AddOrder(&client.OrderDetails{
			ID:          orderID,
			UserID:      "01a16cca-732e-7593-8862-e67348dfccda",
			Status:      "CONFIRMED",
			OrderNumber: "ORD-SAME-CITY",
			ShippingAddress: model.ShippingAddress{
				Street:        "123 Congress Ave",
				City:          "Austin",
				State:         "TX",
				PostalCode:    "78701",
				Country:       "USA",
				ContactNumber: "+15125550101",
			},
		})

		// Estimate delivery via handler API
		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%s/estimate?partner=%s", orderID, partnerID.String()), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var estResp dto.EstimatedDeliveryResponse
		_ = json.Unmarshal(body, &estResp)

		if estResp.DeliveryType != "Same-city delivery" {
			t.Fatalf("expected DeliveryType 'Same-city delivery', got '%s'", estResp.DeliveryType)
		}
		if estResp.MinDays != 1 || estResp.MaxDays != 2 {
			t.Fatalf("expected 1-2 days, got %d-%d", estResp.MinDays, estResp.MaxDays)
		}
		if estResp.TransitEstimate != "1–2 days" {
			t.Fatalf("expected '1–2 days', got '%s'", estResp.TransitEstimate)
		}

		// Also verify shipment creation via event consumer (OrderConfirmed)
		orderConfirmedPayload := map[string]interface{}{
			"order_id":         orderID,
			"delivery_partner": partnerID.String(),
			"contact_number":   "+15125550101",
		}
		data, _ := json.Marshal(orderConfirmedPayload)
		envelope := &events.EventEnvelope{
			EventID:      uuid.NewString(),
			EventType:    events.EventTypeOrderConfirmed,
			AggregateID:  orderID,
			EventVersion: "1.0",
			Source:       "order-service",
			Data:         data,
			Timestamp:    time.Now().UTC(),
		}
		if err := eventConsumer.HandleEvent(ctx, envelope); err != nil {
			t.Fatalf("event consumption failed: %v", err)
		}

		delivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to retrieve delivery: %v", err)
		}
		if delivery.TrackingNumber == "" {
			t.Fatalf("expected tracking number to be set")
		}
		if delivery.EstimatedDelivery == nil {
			t.Fatalf("expected estimated_delivery to be set")
		}
		diffHours := delivery.EstimatedDelivery.Sub(delivery.CreatedAt).Hours()
		if diffHours < 47 || diffHours > 49 {
			t.Fatalf("expected ~48h (2 days) SLA target, got %f hours", diffHours)
		}
	})

	// Case 2: Within the same state: 2–4 days (target: 4 days)
	// Destination: Houston, TX. Origin warehouse: Austin, TX (active)
	t.Run("Within the same state 2-4 days", func(t *testing.T) {
		orderID := uuid.NewString()
		orderClient.AddOrder(&client.OrderDetails{
			ID:          orderID,
			UserID:      "01a16cca-732e-7593-8862-e67348dfccda",
			Status:      "CONFIRMED",
			OrderNumber: "ORD-SAME-STATE",
			ShippingAddress: model.ShippingAddress{
				Street:        "500 Main St",
				City:          "Houston",
				State:         "TX",
				PostalCode:    "77002",
				Country:       "USA",
				ContactNumber: "+17135550202",
			},
		})

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%s/estimate?partner=%s", orderID, partnerID.String()), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var estResp dto.EstimatedDeliveryResponse
		_ = json.Unmarshal(body, &estResp)

		if estResp.DeliveryType != "Within the same state" {
			t.Fatalf("expected DeliveryType 'Within the same state', got '%s'", estResp.DeliveryType)
		}
		if estResp.MinDays != 2 || estResp.MaxDays != 4 {
			t.Fatalf("expected 2-4 days, got %d-%d", estResp.MinDays, estResp.MaxDays)
		}
		if estResp.TransitEstimate != "2–4 days" {
			t.Fatalf("expected '2–4 days', got '%s'", estResp.TransitEstimate)
		}

		// Create delivery and verify target is 4 days
		orderConfirmedPayload := map[string]interface{}{
			"order_id":         orderID,
			"delivery_partner": partnerID.String(),
			"contact_number":   "+17135550202",
		}
		data, _ := json.Marshal(orderConfirmedPayload)
		envelope := &events.EventEnvelope{
			EventID:      uuid.NewString(),
			EventType:    events.EventTypeOrderConfirmed,
			AggregateID:  orderID,
			EventVersion: "1.0",
			Source:       "order-service",
			Data:         data,
			Timestamp:    time.Now().UTC(),
		}
		if err := eventConsumer.HandleEvent(ctx, envelope); err != nil {
			t.Fatalf("event consumption failed: %v", err)
		}

		delivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to retrieve delivery: %v", err)
		}
		diffHours := delivery.EstimatedDelivery.Sub(delivery.CreatedAt).Hours()
		if diffHours < 95 || diffHours > 97 {
			t.Fatalf("expected ~96h (4 days) SLA target, got %f hours", diffHours)
		}
	})

	// Case 3: Inter-state delivery: 4–7 days (target: 7 days)
	// Destination: Chicago, IL. Origin warehouse: Austin, TX (active)
	t.Run("Inter-state delivery 4-7 days", func(t *testing.T) {
		orderID := uuid.NewString()
		orderClient.AddOrder(&client.OrderDetails{
			ID:          orderID,
			UserID:      "01a16cca-732e-7593-8862-e67348dfccda",
			Status:      "CONFIRMED",
			OrderNumber: "ORD-INTER-STATE",
			ShippingAddress: model.ShippingAddress{
				Street:        "200 Michigan Ave",
				City:          "Chicago",
				State:         "IL",
				PostalCode:    "60601",
				Country:       "USA",
				ContactNumber: "+13125550303",
			},
		})

		req := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/orders/%s/estimate?partner=%s", orderID, partnerID.String()), nil)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Fatalf("expected 200, got %d", resp.StatusCode)
		}

		body, _ := io.ReadAll(resp.Body)
		var estResp dto.EstimatedDeliveryResponse
		_ = json.Unmarshal(body, &estResp)

		if estResp.DeliveryType != "Inter-state delivery" {
			t.Fatalf("expected DeliveryType 'Inter-state delivery', got '%s'", estResp.DeliveryType)
		}
		if estResp.MinDays != 4 || estResp.MaxDays != 7 {
			t.Fatalf("expected 4-7 days, got %d-%d", estResp.MinDays, estResp.MaxDays)
		}
		if estResp.TransitEstimate != "4–7 days" {
			t.Fatalf("expected '4–7 days', got '%s'", estResp.TransitEstimate)
		}

		// Create delivery and verify target is 7 days
		orderConfirmedPayload := map[string]interface{}{
			"order_id":         orderID,
			"delivery_partner": partnerID.String(),
			"contact_number":   "+13125550303",
		}
		data, _ := json.Marshal(orderConfirmedPayload)
		envelope := &events.EventEnvelope{
			EventID:      uuid.NewString(),
			EventType:    events.EventTypeOrderConfirmed,
			AggregateID:  orderID,
			EventVersion: "1.0",
			Source:       "order-service",
			Data:         data,
			Timestamp:    time.Now().UTC(),
		}
		if err := eventConsumer.HandleEvent(ctx, envelope); err != nil {
			t.Fatalf("event consumption failed: %v", err)
		}

		delivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
		if err != nil {
			t.Fatalf("failed to retrieve delivery: %v", err)
		}
		diffHours := delivery.EstimatedDelivery.Sub(delivery.CreatedAt).Hours()
		if diffHours < 167 || diffHours > 169 {
			t.Fatalf("expected ~168h (7 days) SLA target, got %f hours", diffHours)
		}

		// Test status updates and actual_delivery recording
		// 1. Transition CREATED -> PICKED_UP
		updated, err := svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusPickedUp,
		})
		if err != nil {
			t.Fatalf("transition to PICKED_UP failed: %v", err)
		}
		if updated.Status != model.StatusPickedUp {
			t.Fatalf("expected status PICKED_UP, got %s", updated.Status)
		}

		// 2. Safe duplicate handling
		dup, err := svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusPickedUp,
		})
		if err != nil {
			t.Fatalf("duplicate update should be safe: %v", err)
		}
		if dup.Status != model.StatusPickedUp {
			t.Fatalf("expected status PICKED_UP, got %s", dup.Status)
		}

		// 3. Reject invalid transition (PICKED_UP -> DELIVERED directly is disallowed by state machine)
		_, err = svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusDelivered,
		})
		if err == nil {
			t.Fatalf("expected invalid state transition error")
		}

		// 4. Proper progression: PICKED_UP -> IN_TRANSIT -> OUT_FOR_DELIVERY -> DELIVERED
		_, err = svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusInTransit,
		})
		if err != nil {
			t.Fatalf("transition to IN_TRANSIT failed: %v", err)
		}

		_, err = svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusOutForDelivery,
		})
		if err != nil {
			t.Fatalf("transition to OUT_FOR_DELIVERY failed: %v", err)
		}

		// Now DELIVERED: should automatically record actual_delivery
		delivered, err := svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.StatusDelivered,
		})
		if err != nil {
			t.Fatalf("transition to DELIVERED failed: %v", err)
		}
		if delivered.Status != model.StatusDelivered {
			t.Fatalf("expected DELIVERED, got %s", delivered.Status)
		}
		if delivered.ActualDelivery == nil {
			t.Fatalf("expected actual_delivery to be recorded when delivered")
		}

		// 5. Reject unknown status
		_, err = svc.ProcessStatusUpdate(ctx, &dto.DeliveryStatusUpdateRequest{
			DeliveryID: delivery.ID,
			Status:     model.DeliveryStatus("SOMETHING_INVALID"),
		})
		if err == nil {
			t.Fatalf("expected error for unknown status")
		}
	})

	t.Run("Same-city delivery from comma-separated street and company name Rapido in Madurai", func(t *testing.T) {
		rapidoID := uuid.New()
		_ = repo.CreateOrUpdatePartnerProfile(ctx, &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   rapidoID,
			CompanyName: "Rapido",
			IsActive:    true,
		})
		_ = repo.CreateWarehouse(ctx, &model.Warehouse{
			ID:                uuid.New(),
			DeliveryPartnerID: rapidoID,
			Name:              "Rapido Madurai Hub",
			Code:              "RAP-MDU-01",
			City:              "Madurai",
			State:             "Tamil Nadu",
			IsActive:          true,
		})

		orderID := uuid.NewString()
		parsedAddr := model.ParseShippingAddress("24 satya sai nagar , tvs nagar ,madurai")
		parsedAddr.ContactNumber = "6383449176"

		now := time.Now().UTC()
		delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
			OrderID:         orderID,
			DeliveryPartner: "Rapido",
			ShippingAddress: &parsedAddr,
		})
		if err != nil {
			t.Fatalf("HandleOrderConfirmed failed: %v", err)
		}
		if delivery == nil {
			t.Fatal("expected delivery not to be nil")
		}
		if delivery.EstimatedDelivery == nil {
			t.Fatal("expected estimated_delivery to be set")
		}

		diffHours := delivery.EstimatedDelivery.Sub(now).Hours()
		// Same-city delivery should target 2 days (48 hours), NOT 7 days (168 hours)
		if diffHours < 40 || diffHours > 56 {
			t.Fatalf("expected estimated delivery ~48 hours (1-2 days), got %.1f hours", diffHours)
		}
		if delivery.DeliveryPartner != "Rapido" {
			t.Fatalf("expected partner 'Rapido', got '%s'", delivery.DeliveryPartner)
		}
	})
}
