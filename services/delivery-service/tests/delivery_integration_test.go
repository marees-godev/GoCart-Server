package tests

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
	"github.com/marees-godev/GoCart-Server/contracts/events"
	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	deliveryGRPC "github.com/marees-godev/GoCart-Server/services/delivery-service/internal/grpc"
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

	if _, ok := r.byOrderID[delivery.OrderID]; ok {
		return nil
	}

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
		if delID, okOrder := r.byOrderID[id]; okOrder {
			d = r.deliveries[delID]
		} else {
			return nil, appErrors.NotFound("delivery not found")
		}
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
		if delID, okOrder := r.byOrderID[id]; okOrder {
			d = r.deliveries[delID]
		} else {
			return nil, appErrors.NotFound("delivery not found")
		}
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

func (r *memoryDeliveryRepo) GetPartnerProfileByGSTIN(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	clean := strings.ToUpper(strings.TrimSpace(gstin))
	for _, p := range r.partnerProfiles {
		if p.GSTIN != nil && strings.ToUpper(strings.TrimSpace(*p.GSTIN)) == clean && p.DeletedAt == nil {
			cp := *p
			return &cp, nil
		}
	}
	return nil, appErrors.NotFound("delivery partner profile not found")
}

func (r *memoryDeliveryRepo) ListAppealedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*model.DeliveryPartnerProfile, 0)
	for _, p := range r.partnerProfiles {
		if p.ApprovalStatus == model.ApprovalStatusAppealed && p.DeletedAt == nil {
			cp := *p
			res = append(res, &cp)
		}
	}
	return res, len(res), nil
}

func (r *memoryDeliveryRepo) ListApprovedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	res := make([]*model.DeliveryPartnerProfile, 0)
	for _, p := range r.partnerProfiles {
		if p.ApprovalStatus == model.ApprovalStatusApproved && p.IsActive && p.DeletedAt == nil {
			cp := *p
			res = append(res, &cp)
		}
	}
	return res, len(res), nil
}

func (r *memoryDeliveryRepo) SoftDeletePartnerProfile(ctx context.Context, partnerID uuid.UUID, deletedAt time.Time, reason ...string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	p, ok := r.partnerProfiles[partnerID]
	if !ok || p.DeletedAt != nil {
		return appErrors.NotFound("partner profile not found")
	}
	p.DeletedAt = &deletedAt
	p.IsActive = false
	if len(reason) > 0 && reason[0] != "" {
		p.DeletionReason = &reason[0]
	}
	return nil
}

func (r *memoryDeliveryRepo) SoftDeleteExpiredDeactivatedPartners(ctx context.Context, cutoff time.Time) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	now := time.Now().UTC()
	for _, p := range r.partnerProfiles {
		if !p.IsActive && p.DeactivatedAt != nil && (p.DeactivatedAt.Before(cutoff) || p.DeactivatedAt.Equal(cutoff)) && p.DeletedAt == nil {
			p.DeletedAt = &now
			count++
		}
	}
	return count, nil
}




func TestEndToEnd_DeliveryLifecycleAndAPIs(t *testing.T) {
	ctx := context.Background()
	repo := newMemoryDeliveryRepo()
	svc := service.NewDeliveryService(repo)
	eventConsumer := consumer.NewDeliveryEventConsumer(svc)

	app := fiber.New()
	httpHandler := handler.NewDeliveryHandler(svc)
	httpHandler.RegisterRoutes(app)

	grpcHandler := deliveryGRPC.NewDeliveryGRPCServer(svc)

	orderID := "ord-e2e-" + uuid.NewString()
	shippingAddr := events.ShippingAddressPayload{
		Street:        "500 Howard Street",
		City:          "San Francisco",
		State:         "CA",
		PostalCode:    "94105",
		Country:       "United States",
		ContactNumber: "+1-415-555-0142",
	}

	payload := events.OrderConfirmedEvent{
		OrderID:           orderID,
		ShippingAddressID: "addr-e2e-1",
		ShippingAddress:   &shippingAddr,
		ConfirmedAt:       time.Now().UTC(),
	}

	// 1. Emit OrderConfirmed event
	env1, err := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	if err := eventConsumer.HandleEvent(ctx, env1); err != nil {
		t.Fatalf("failed to consume OrderConfirmed event: %v", err)
	}

	// 2. Assert CREATED delivery record is saved
	delivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
	if err != nil {
		t.Fatalf("delivery not found after event: %v", err)
	}
	if delivery.OrderID != orderID {
		t.Errorf("expected orderID %s, got %s", orderID, delivery.OrderID)
	}
	if delivery.Status != model.StatusCreated {
		t.Errorf("expected initial status CREATED, got %s", delivery.Status)
	}
	if delivery.DeliveryPartner == "" {
		t.Errorf("expected delivery partner to be set")
	}
	if delivery.TrackingNumber == "" {
		t.Errorf("expected tracking number to be generated")
	}
	if delivery.ShippingAddress.Street != shippingAddr.Street {
		t.Errorf("expected street %s, got %s", shippingAddr.Street, delivery.ShippingAddress.Street)
	}
	if delivery.ShippingAddress.PostalCode != shippingAddr.PostalCode {
		t.Errorf("expected postal code %s, got %s", shippingAddr.PostalCode, delivery.ShippingAddress.PostalCode)
	}
	if delivery.ShippingAddress.ContactNumber != shippingAddr.ContactNumber {
		t.Errorf("expected contact number %s, got %s", shippingAddr.ContactNumber, delivery.ShippingAddress.ContactNumber)
	}
	if delivery.ShippingAddressID != "addr-e2e-1" {
		t.Errorf("expected shippingAddressID 'addr-e2e-1', got %s", delivery.ShippingAddressID)
	}

	// 3. Emit duplicate OrderConfirmed events for the same order_id (idempotency assertion)
	env2, _ := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err := eventConsumer.HandleEvent(ctx, env2); err != nil {
		t.Fatalf("duplicate event consumption failed: %v", err)
	}
	env3, _ := events.NewEventEnvelope(events.EventTypeOrderConfirmed, "order-service", payload)
	if err := eventConsumer.HandleEvent(ctx, env3); err != nil {
		t.Fatalf("duplicate event consumption failed: %v", err)
	}

	dupCheckDelivery, err := svc.GetDeliveryByOrderID(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to query delivery after duplicates: %v", err)
	}
	if dupCheckDelivery.ID != delivery.ID {
		t.Errorf("duplicate event created second delivery: %s != %s", dupCheckDelivery.ID, delivery.ID)
	}
	if dupCheckDelivery.TrackingNumber != delivery.TrackingNumber {
		t.Errorf("duplicate event changed tracking number: %s != %s", dupCheckDelivery.TrackingNumber, delivery.TrackingNumber)
	}

	// 4. Query Customer Tracking API (GET /orders/:orderId/tracking)
	req := httptest.NewRequest(http.MethodGet, "/orders/"+orderID+"/tracking", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("GET /orders/:orderId/tracking failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	var trackingResp dto.TrackingResponse
	if err := json.Unmarshal(body, &trackingResp); err != nil {
		t.Fatalf("failed to decode tracking response: %v", err)
	}

	if trackingResp.OrderID != orderID {
		t.Errorf("expected order_id %s, got %s", orderID, trackingResp.OrderID)
	}
	if trackingResp.TrackingNumber != delivery.TrackingNumber {
		t.Errorf("expected tracking_number %s, got %s", delivery.TrackingNumber, trackingResp.TrackingNumber)
	}
	if trackingResp.Status != model.StatusCreated {
		t.Errorf("expected status CREATED, got %s", trackingResp.Status)
	}
	if trackingResp.ShippingAddress.Street != shippingAddr.Street {
		t.Errorf("expected street %s, got %s", shippingAddr.Street, trackingResp.ShippingAddress.Street)
	}
	if trackingResp.ShippingAddress.ContactNumber != shippingAddr.ContactNumber {
		t.Errorf("expected contact number %s, got %s", shippingAddr.ContactNumber, trackingResp.ShippingAddress.ContactNumber)
	}
	if trackingResp.ShippingAddressID != "addr-e2e-1" {
		t.Errorf("expected shippingAddressID 'addr-e2e-1', got %s", trackingResp.ShippingAddressID)
	}

	// 5. Query Tracking by Tracking Number (GET /deliveries/:trackingNumber)
	reqTrk := httptest.NewRequest(http.MethodGet, "/deliveries/"+delivery.TrackingNumber, nil)
	respTrk, err := app.Test(reqTrk)
	if err != nil {
		t.Fatalf("GET /deliveries/:trackingNumber failed: %v", err)
	}
	defer respTrk.Body.Close()
	if respTrk.StatusCode != http.StatusOK {
		t.Fatalf("expected status 200, got %d", respTrk.StatusCode)
	}

	// 6. Test state machine updates via PATCH /deliveries/:id/status
	// Transition 1: CREATED -> PICKED_UP
	patchBody1, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusPickedUp})
	reqPatch1 := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBody1))
	reqPatch1.Header.Set("Content-Type", "application/json")
	respPatch1, err := app.Test(reqPatch1)
	if err != nil || respPatch1.StatusCode != http.StatusOK {
		t.Fatalf("transition to PICKED_UP failed: %v (status: %d)", err, respPatch1.StatusCode)
	}
	respPatch1.Body.Close()

	// Invalid Transition: PICKED_UP -> DELIVERED (skipping intermediate states)
	patchBodyInvalid, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusDelivered})
	reqPatchInvalid := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBodyInvalid))
	reqPatchInvalid.Header.Set("Content-Type", "application/json")
	respPatchInvalid, err := app.Test(reqPatchInvalid)
	if err != nil || respPatchInvalid.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for skipping state, got %d", respPatchInvalid.StatusCode)
	}
	respPatchInvalid.Body.Close()

	// Transition 2: PICKED_UP -> IN_TRANSIT
	patchBody2, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusInTransit})
	reqPatch2 := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBody2))
	reqPatch2.Header.Set("Content-Type", "application/json")
	respPatch2, _ := app.Test(reqPatch2)
	if respPatch2.StatusCode != http.StatusOK {
		t.Fatalf("transition to IN_TRANSIT failed: %d", respPatch2.StatusCode)
	}
	respPatch2.Body.Close()

	// Transition 3: IN_TRANSIT -> OUT_FOR_DELIVERY
	patchBody3, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusOutForDelivery})
	reqPatch3 := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBody3))
	reqPatch3.Header.Set("Content-Type", "application/json")
	respPatch3, _ := app.Test(reqPatch3)
	if respPatch3.StatusCode != http.StatusOK {
		t.Fatalf("transition to OUT_FOR_DELIVERY failed: %d", respPatch3.StatusCode)
	}
	respPatch3.Body.Close()

	// Transition 4: OUT_FOR_DELIVERY -> DELIVERED
	patchBody4, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusDelivered})
	reqPatch4 := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBody4))
	reqPatch4.Header.Set("Content-Type", "application/json")
	respPatch4, _ := app.Test(reqPatch4)
	if respPatch4.StatusCode != http.StatusOK {
		t.Fatalf("transition to DELIVERED failed: %d", respPatch4.StatusCode)
	}
	respPatch4.Body.Close()

	// Attempt transition out of terminal state DELIVERED -> IN_TRANSIT
	patchBodyTerminal, _ := json.Marshal(dto.UpdateStatusRequest{Status: model.StatusInTransit})
	reqPatchTerminal := httptest.NewRequest(http.MethodPatch, "/deliveries/"+delivery.ID+"/status", bytes.NewReader(patchBodyTerminal))
	reqPatchTerminal.Header.Set("Content-Type", "application/json")
	respPatchTerminal, _ := app.Test(reqPatchTerminal)
	if respPatchTerminal.StatusCode != http.StatusUnprocessableEntity {
		t.Fatalf("expected 422 for terminal state transition out of DELIVERED, got %d", respPatchTerminal.StatusCode)
	}
	respPatchTerminal.Body.Close()

	// 7. Verify gRPC endpoints work seamlessly
	grpcResp, err := grpcHandler.GetDeliveryByOrder(ctx, &deliverypb.GetDeliveryByOrderRequest{
		OrderId: orderID,
	})
	if err != nil {
		t.Fatalf("gRPC GetDeliveryByOrder failed: %v", err)
	}
	if grpcResp.Delivery.Status != string(model.StatusDelivered) {
		t.Errorf("expected final gRPC status DELIVERED, got %s", grpcResp.Delivery.Status)
	}
	if grpcResp.Delivery.ContactNumber != shippingAddr.ContactNumber {
		t.Errorf("expected gRPC contact_number %s, got %s", shippingAddr.ContactNumber, grpcResp.Delivery.ContactNumber)
	}
	if grpcResp.Delivery.ShippingAddressId != "addr-e2e-1" {
		t.Errorf("expected gRPC shipping_address_id 'addr-e2e-1', got %s", grpcResp.Delivery.ShippingAddressId)
	}

	// 8. Verify lookup and status updates via Order ID work seamlessly
	orderIDResp, err := grpcHandler.GetDelivery(ctx, &deliverypb.GetDeliveryRequest{
		Id: orderID,
	})
	if err != nil {
		t.Fatalf("gRPC GetDelivery by order_id failed: %v", err)
	}
	if orderIDResp.Delivery.Id != delivery.ID {
		t.Errorf("expected resolved delivery ID %s, got %s", delivery.ID, orderIDResp.Delivery.Id)
	}

	// 9. Verify idempotent status update via gRPC succeeds
	sameStatusResp, err := grpcHandler.UpdateDeliveryStatus(ctx, &deliverypb.UpdateDeliveryStatusRequest{
		Id:     orderID,
		Status: string(model.StatusDelivered),
	})
	if err != nil {
		t.Fatalf("idempotent gRPC UpdateDeliveryStatus by order_id failed: %v", err)
	}
	if sameStatusResp.Delivery.Status != string(model.StatusDelivered) {
		t.Errorf("expected status DELIVERED, got %s", sameStatusResp.Delivery.Status)
	}

	// 10. Verify gRPC CreateDelivery requires shipping_address_id and succeeds when provided
	_, createErrNoAddr := grpcHandler.CreateDelivery(ctx, &deliverypb.CreateDeliveryRequest{
		OrderId:         "ord-grpc-" + uuid.NewString(),
		DeliveryPartner: "DELHIVERY",
		ShippingAddress: `{"street":"100 Pine St","city":"San Francisco"}`,
		ContactNumber:   "+1-555-0100",
	})
	if createErrNoAddr == nil {
		t.Fatal("expected error when CreateDelivery is called without shipping_address_id")
	}

	newOrderID := "ord-grpc-" + uuid.NewString()
	createResp, createErr := grpcHandler.CreateDelivery(ctx, &deliverypb.CreateDeliveryRequest{
		OrderId:           newOrderID,
		DeliveryPartner:   "DELHIVERY",
		ShippingAddress:   `{"street":"100 Pine St","city":"San Francisco"}`,
		ContactNumber:     "+1-555-0100",
		ShippingAddressId: "addr-e2e-grpc",
	})
	if createErr != nil {
		t.Fatalf("CreateDelivery with shipping_address_id failed: %v", createErr)
	}
	if createResp.Delivery.ShippingAddressId != "addr-e2e-grpc" {
		t.Errorf("expected shipping_address_id 'addr-e2e-grpc', got %s", createResp.Delivery.ShippingAddressId)
	}
}
