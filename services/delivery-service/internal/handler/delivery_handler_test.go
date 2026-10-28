package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/service"
)

type memoryDeliveryRepo struct {
	mu                sync.RWMutex
	deliveries        map[string]*model.Delivery
	byOrderID         map[string]string
	byTracking        map[string]string
	warehouses        map[uuid.UUID]*model.Warehouse
	partnerProfiles   map[uuid.UUID]*model.DeliveryPartnerProfile
	partnerWarehouses map[uuid.UUID][]uuid.UUID
}

func newMemoryDeliveryRepo() repository.DeliveryRepository {
	return &memoryDeliveryRepo{
		deliveries:        make(map[string]*model.Delivery),
		byOrderID:         make(map[string]string),
		byTracking:        make(map[string]string),
		warehouses:        make(map[uuid.UUID]*model.Warehouse),
		partnerProfiles:   make(map[uuid.UUID]*model.DeliveryPartnerProfile),
		partnerWarehouses: make(map[uuid.UUID][]uuid.UUID),
	}
}

func (r *memoryDeliveryRepo) Create(ctx context.Context, delivery *model.Delivery) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	if delivery.ID == "" {
		delivery.ID = uuid.NewString()
	}
	now := time.Now().UTC()
	if delivery.CreatedAt.IsZero() {
		delivery.CreatedAt = now
	}
	if delivery.UpdatedAt.IsZero() {
		delivery.UpdatedAt = now
	}

	stored := *delivery
	r.deliveries[delivery.ID] = &stored
	r.byOrderID[delivery.OrderID] = delivery.ID
	r.byTracking[delivery.TrackingNumber] = delivery.ID
	return nil
}

func (r *memoryDeliveryRepo) GetByID(ctx context.Context, id string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	d, ok := r.deliveries[id]
	if !ok {
		return nil, appErrors.NotFound("delivery not found")
	}
	copyD := *d
	return &copyD, nil
}

func (r *memoryDeliveryRepo) GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byOrderID[orderID]
	if !ok {
		return nil, appErrors.NotFound("delivery not found for order")
	}
	return r.GetByID(ctx, id)
}

func (r *memoryDeliveryRepo) GetByTrackingNumber(ctx context.Context, trackingNumber string) (*model.Delivery, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	id, ok := r.byTracking[trackingNumber]
	if !ok {
		return nil, appErrors.NotFound("delivery not found for tracking number")
	}
	return r.GetByID(ctx, id)
}

func (r *memoryDeliveryRepo) List(ctx context.Context, limit, offset int, status string) ([]*model.Delivery, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.Delivery
	for _, d := range r.deliveries {
		if status == "" || string(d.Status) == status {
			copyD := *d
			result = append(result, &copyD)
		}
	}
	return result, len(result), nil
}

func (r *memoryDeliveryRepo) UpdateStatus(ctx context.Context, id string, status model.DeliveryStatus) (*model.Delivery, error) {
	return r.UpdateStatusWithDetails(ctx, id, status, nil, nil, "", "")
}

func (r *memoryDeliveryRepo) UpdateStatusWithDetails(ctx context.Context, id string, status model.DeliveryStatus, estimatedDelivery, actualDelivery *time.Time, location, notes string) (*model.Delivery, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	d, ok := r.deliveries[id]
	if !ok {
		return nil, appErrors.NotFound("delivery not found")
	}

	cleanStatus := model.DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(status))))
	if !model.IsValidStatus(cleanStatus) {
		return nil, appErrors.BadRequest("invalid or unknown delivery status: " + string(status))
	}

	if d.Status == cleanStatus {
		if estimatedDelivery != nil {
			d.EstimatedDelivery = estimatedDelivery
		}
		if actualDelivery != nil {
			d.ActualDelivery = actualDelivery
		}
		copyD := *d
		return &copyD, nil
	}

	if err := d.TransitionStatus(cleanStatus); err != nil {
		return nil, err
	}

	if estimatedDelivery != nil {
		d.EstimatedDelivery = estimatedDelivery
	}
	if cleanStatus == model.StatusDelivered {
		if actualDelivery != nil {
			d.ActualDelivery = actualDelivery
		} else if d.ActualDelivery == nil {
			now := time.Now().UTC()
			d.ActualDelivery = &now
		}
	} else if actualDelivery != nil {
		d.ActualDelivery = actualDelivery
	}

	d.Status = cleanStatus
	d.UpdatedAt = time.Now().UTC()
	copyD := *d
	return &copyD, nil
}

func (r *memoryDeliveryRepo) GetAvailableWarehouses(ctx context.Context) ([]*model.Warehouse, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*model.Warehouse, 0, len(r.warehouses))
	for _, w := range r.warehouses {
		if w.IsActive {
			cp := *w
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (r *memoryDeliveryRepo) GetWarehouseByID(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) (*model.Warehouse, error) {
	if partnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	w, ok := r.warehouses[id]
	if !ok || w.DeliveryPartnerID != partnerID {
		return nil, appErrors.NotFound("warehouse not found")
	}
	cp := *w
	return &cp, nil
}




func (r *memoryDeliveryRepo) GetWarehousesByPartnerID(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	if partnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*model.Warehouse, 0)
	for _, w := range r.warehouses {
		if w.DeliveryPartnerID == partnerID {
			cp := *w
			res = append(res, &cp)
		}
	}
	return res, nil
}

func (r *memoryDeliveryRepo) GetPartnerWarehouses(ctx context.Context, partnerID uuid.UUID) ([]*model.Warehouse, error) {
	return r.GetWarehousesByPartnerID(ctx, partnerID)
}

func (r *memoryDeliveryRepo) CreateWarehouse(ctx context.Context, w *model.Warehouse) error {
	if w.DeliveryPartnerID == uuid.Nil {
		return appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if w.ID == uuid.Nil {
		w.ID = uuid.New()
	}
	cp := *w
	r.warehouses[w.ID] = &cp
	return nil
}

func (r *memoryDeliveryRepo) UpdateWarehouse(ctx context.Context, w *model.Warehouse) (*model.Warehouse, error) {
	if w.DeliveryPartnerID == uuid.Nil {
		return nil, appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.warehouses[w.ID]
	if !ok || existing.DeliveryPartnerID != w.DeliveryPartnerID {
		return nil, appErrors.NotFound("warehouse not found")
	}
	cp := *w
	r.warehouses[w.ID] = &cp
	return &cp, nil
}




func (r *memoryDeliveryRepo) DeleteWarehouse(ctx context.Context, partnerID uuid.UUID, id uuid.UUID) error {
	if partnerID == uuid.Nil {
		return appErrors.BadRequest("delivery_partner_id is mandatory to proceed")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	existing, ok := r.warehouses[id]
	if !ok || existing.DeliveryPartnerID != partnerID {
		return appErrors.NotFound("warehouse not found")
	}
	delete(r.warehouses, id)
	return nil
}

func (r *memoryDeliveryRepo) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if profile.ID == uuid.Nil {
		profile.ID = uuid.New()
	}
	cp := *profile
	r.partnerProfiles[profile.PartnerID] = &cp
	return nil
}

func (r *memoryDeliveryRepo) GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	p, ok := r.partnerProfiles[partnerID]
	if !ok {
		return nil, appErrors.NotFound("partner profile not found")
	}
	cp := *p
	return &cp, nil
}

func (r *memoryDeliveryRepo) GetPartnerProfileByCompanyName(ctx context.Context, name string) (*model.DeliveryPartnerProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clean := strings.ToLower(strings.TrimSpace(name))
	for _, p := range r.partnerProfiles {
		if strings.ToLower(strings.TrimSpace(p.CompanyName)) == clean {
			cp := *p
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("partner profile not found")
}




func setupTestApp(t *testing.T) (*fiber.App, service.DeliveryService) {
	t.Helper()
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)

	app := fiber.New()
	h := handler.NewDeliveryHandler(svc)
	h.RegisterRoutes(app)

	return app, svc
}

func TestCustomerTrackingEndpoint_ByOrderID(t *testing.T) {
	app, svc := setupTestApp(t)
	ctx := context.Background()

	orderID := "ord-" + uuid.NewString()
	delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
		OrderID:         orderID,
		DeliveryPartner: "FEDEX",
		ShippingAddress: &model.ShippingAddress{
			Street:        "789 Pine St",
			City:          "Portland",
			State:         "OR",
			PostalCode:    "97201",
			Country:       "USA",
			ContactNumber: "+1-555-0123",
		},
	})
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID+"/tracking", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var trackingResp dto.TrackingResponse
	if err := json.Unmarshal(body, &trackingResp); err != nil {
		t.Fatalf("failed to unmarshal tracking response: %v", err)
	}

	if trackingResp.OrderID != orderID {
		t.Errorf("expected order_id %s, got %s", orderID, trackingResp.OrderID)
	}
	if trackingResp.TrackingNumber != delivery.TrackingNumber {
		t.Errorf("expected tracking_number %s, got %s", delivery.TrackingNumber, trackingResp.TrackingNumber)
	}
	if trackingResp.DeliveryPartner != "FEDEX" {
		t.Errorf("expected delivery_partner 'FEDEX', got %s", trackingResp.DeliveryPartner)
	}
	if trackingResp.Status != model.StatusCreated {
		t.Errorf("expected status 'CREATED', got %s", trackingResp.Status)
	}
	if trackingResp.ShippingAddress.Street != "789 Pine St" {
		t.Errorf("expected street '789 Pine St', got %s", trackingResp.ShippingAddress.Street)
	}
	if trackingResp.ShippingAddress.ContactNumber != "+1-555-0123" {
		t.Errorf("expected contact_number '+1-555-0123', got %s", trackingResp.ShippingAddress.ContactNumber)
	}
	if trackingResp.UpdatedAt == "" {
		t.Errorf("expected updated_at to be non-empty")
	}
}

func TestCustomerTrackingEndpoint_ByTrackingNumber(t *testing.T) {
	app, svc := setupTestApp(t)
	ctx := context.Background()

	orderID := "ord-" + uuid.NewString()
	delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
		OrderID:         orderID,
		DeliveryPartner: "DHL",
		ShippingAddress: &model.ShippingAddress{
			Street:        "100 Broadway",
			City:          "New York",
			State:         "NY",
			PostalCode:    "10005",
			Country:       "USA",
			ContactNumber: "+1-555-0199",
		},
	})
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/deliveries/"+delivery.TrackingNumber, nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to send request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var trackingResp dto.TrackingResponse
	if err := json.Unmarshal(body, &trackingResp); err != nil {
		t.Fatalf("failed to unmarshal tracking response: %v", err)
	}

	if trackingResp.TrackingNumber != delivery.TrackingNumber {
		t.Errorf("expected tracking_number %s, got %s", delivery.TrackingNumber, trackingResp.TrackingNumber)
	}
	if trackingResp.DeliveryPartner != "DHL" {
		t.Errorf("expected delivery_partner 'DHL', got %s", trackingResp.DeliveryPartner)
	}
}

func TestCustomerTrackingEndpoint_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	// Non-existent order tracking
	req1 := httptest.NewRequest(http.MethodGet, "/orders/non-existent-order-id/tracking", nil)
	resp1, err := app.Test(req1)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp1.Body.Close()
	if resp1.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent order, got %d", resp1.StatusCode)
	}

	// Non-existent tracking number
	req2 := httptest.NewRequest(http.MethodGet, "/deliveries/NON_EXISTENT_TRK_NUM", nil)
	resp2, err := app.Test(req2)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for non-existent tracking number, got %d", resp2.StatusCode)
	}
}

func TestDeliveryStatusUpdate_ValidTransition(t *testing.T) {
	app, svc := setupTestApp(t)
	ctx := context.Background()

	orderID := "ord-" + uuid.NewString()
	delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
		OrderID: orderID,
		ShippingAddress: &model.ShippingAddress{
			Street:        "123 Elm St",
			City:          "Boston",
			State:         "MA",
			PostalCode:    "02108",
			Country:       "USA",
			ContactNumber: "+1-555-0101",
		},
	})
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	// Update CREATED -> PICKED_UP
	updateBody, _ := json.Marshal(dto.UpdateStatusRequest{
		Status: model.StatusPickedUp,
	})
	req := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute PATCH request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200 OK, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var respMap map[string]any
	if err := json.Unmarshal(body, &respMap); err != nil {
		t.Fatalf("failed to unmarshal JSON response: %v", err)
	}

	if respMap["status"] != string(model.StatusPickedUp) {
		t.Errorf("expected status PICKED_UP, got %v", respMap["status"])
	}
}

func TestDeliveryStatusUpdate_DisallowedTransition_Rejected(t *testing.T) {
	app, svc := setupTestApp(t)
	ctx := context.Background()

	orderID := "ord-" + uuid.NewString()
	delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
		OrderID: orderID,
		ShippingAddress: &model.ShippingAddress{
			Street:        "123 Elm St",
			City:          "Boston",
			State:         "MA",
			PostalCode:    "02108",
			Country:       "USA",
			ContactNumber: "+1-555-0102",
		},
	})
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	// Disallowed: CREATED -> DELIVERED (skipping intermediate states)
	updateBody, _ := json.Marshal(dto.UpdateStatusRequest{
		Status: model.StatusDelivered,
	})
	req := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute PATCH request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 Unprocessable Entity, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var errResp map[string]any
	if err := json.Unmarshal(body, &errResp); err != nil {
		t.Fatalf("failed to unmarshal error response: %v", err)
	}

	errDetails, ok := errResp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected 'error' object in response, got %v", errResp)
	}

	if errDetails["code"] != appErrors.CodeUnprocessableEntity {
		t.Errorf("expected code UNPROCESSABLE_ENTITY, got %v", errDetails["code"])
	}
}

func TestDeliveryStatusUpdate_TerminalState_Rejected(t *testing.T) {
	app, svc := setupTestApp(t)
	ctx := context.Background()

	orderID := "ord-" + uuid.NewString()
	delivery, err := svc.HandleOrderConfirmed(ctx, &dto.OrderConfirmedPayload{
		OrderID: orderID,
		ShippingAddress: &model.ShippingAddress{
			Street:        "123 Elm St",
			City:          "Boston",
			State:         "MA",
			PostalCode:    "02108",
			Country:       "USA",
			ContactNumber: "+1-555-0103",
		},
	})
	if err != nil {
		t.Fatalf("failed to create delivery: %v", err)
	}

	// Transition to FAILED (terminal state)
	_, err = svc.UpdateStatus(ctx, delivery.ID, model.StatusFailed)
	if err != nil {
		t.Fatalf("failed to transition to FAILED: %v", err)
	}

	// Attempt transition out of FAILED -> DELIVERED
	updateBody, _ := json.Marshal(dto.UpdateStatusRequest{
		Status: model.StatusDelivered,
	})
	req := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute PATCH request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected status 422 for terminal state transition, got %d", resp.StatusCode)
	}
}

func TestDeliveryStatusUpdate_NotFound(t *testing.T) {
	app, _ := setupTestApp(t)

	updateBody, _ := json.Marshal(dto.UpdateStatusRequest{
		Status: model.StatusPickedUp,
	})
	req := httptest.NewRequest(http.MethodPatch, "/deliveries/non-existent-id/status", bytes.NewReader(updateBody))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to execute request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Fatalf("expected status 404 for non-existent delivery, got %d", resp.StatusCode)
	}
}
