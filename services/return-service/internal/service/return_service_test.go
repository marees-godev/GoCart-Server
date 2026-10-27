package service_test

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
)

type mockReturnRepo struct {
	mu           sync.RWMutex
	returns      map[string]*model.Return
	byOrderItem  map[string]*model.Return
	outboxEvents []*outbox.Event
}

func newMockReturnRepo() *mockReturnRepo {
	return &mockReturnRepo{
		returns:      make(map[string]*model.Return),
		byOrderItem:  make(map[string]*model.Return),
		outboxEvents: make([]*outbox.Event, 0),
	}
}

func (m *mockReturnRepo) CreateReturnWithOutbox(ctx context.Context, ret *model.Return, outboxEvent *outbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *ret
	m.returns[ret.ID] = &cp
	m.byOrderItem[ret.OrderItemID] = &cp
	if outboxEvent != nil {
		m.outboxEvents = append(m.outboxEvents, outboxEvent)
	}
	return nil
}

func (m *mockReturnRepo) GetReturnByID(ctx context.Context, id string) (*model.Return, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.returns[id]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	cp := *ret
	return &cp, nil
}

func (m *mockReturnRepo) GetReturnByOrderItemID(ctx context.Context, orderItemID string) (*model.Return, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.byOrderItem[orderItemID]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	cp := *ret
	return &cp, nil
}

func (m *mockReturnRepo) ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Return
	for _, r := range m.returns {
		if r.OrderID == orderID {
			cp := *r
			list = append(list, &cp)
		}
	}
	return list, nil
}

func (m *mockReturnRepo) UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret, ok := m.returns[id]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	ret.Status = status
	ret.UpdatedAt = time.Now().UTC()
	cp := *ret
	return &cp, nil
}


func (m *mockReturnRepo) ListReturns(ctx context.Context, limit, offset int, status, userID, returnType string) ([]*model.Return, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var list []*model.Return
	for _, r := range m.returns {
		if status != "" && string(r.Status) != status {
			continue
		}
		if userID != "" && r.UserID != userID {
			continue
		}
		if returnType != "" && string(r.Type) != returnType {
			continue
		}
		cp := *r
		list = append(list, &cp)
	}
	return list, len(list), nil
}

func (m *mockReturnRepo) CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvent *outbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret, ok := m.returns[v.ReturnID]
	if !ok {
		return repository.ErrReturnNotFound
	}
	ret.Status = newStatus
	ret.Verification = v
	ret.UpdatedAt = time.Now().UTC()
	if outboxEvent != nil {
		m.outboxEvents = append(m.outboxEvents, outboxEvent)
	}
	return nil
}

func (m *mockReturnRepo) GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.returns[returnID]
	if !ok || ret.Verification == nil {
		return nil, nil
	}
	cp := *ret.Verification
	return &cp, nil
}


type mockOrderClient struct {
	order *orderpb.Order
	err   error
}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID, userID string) (*orderpb.Order, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.order, nil
}

type mockProductClient struct {
	product *productpb.Product
	err     error
}

func (m *mockProductClient) GetProduct(ctx context.Context, productID string) (*productpb.Product, error) {
	if m.err != nil {
		return nil, m.err
	}
	return m.product, nil
}

func validDeliveredOrder(orderID, userID, itemID, productID string, updatedAt time.Time) *orderpb.Order {
	return &orderpb.Order{
		Id:        orderID,
		UserId:    userID,
		Status:    "DELIVERED",
		UpdatedAt: updatedAt.Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{
				Id:         itemID,
				ProductId:  productID,
				Quantity:   2,
				UnitPrice:  50.0,
				TotalPrice: 100.0,
			},
		},
	}
}

func TestReturnService_RequestReturn_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC().Add(-2*24*time.Hour)),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{
			Id:           productID,
			IsReturnable: true,
		},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)

	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Defective item received",
	}

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected no error, got: %v", err)
	}

	if res.ID == "" {
		t.Errorf("expected return ID to be set")
	}
	if res.Status != model.ReturnStatusRequested {
		t.Errorf("expected status %s, got %s", model.ReturnStatusRequested, res.Status)
	}
	if res.RefundAmount != 100.0 {
		t.Errorf("expected refund amount 100.0, got %f", res.RefundAmount)
	}
	if res.UserID != userID {
		t.Errorf("expected user ID %s, got %s", userID, res.UserID)
	}

	// Verify persistence in repository
	saved, err := repo.GetReturnByID(ctx, res.ID)
	if err != nil {
		t.Fatalf("failed to retrieve persisted return: %v", err)
	}
	if saved.ID != res.ID {
		t.Errorf("expected saved ID %s, got %s", res.ID, saved.ID)
	}

	// Verify ReturnRequested domain event published to outbox
	if len(repo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(repo.outboxEvents))
	}
	evt := repo.outboxEvents[0]
	if evt.EventType != events.EventTypeReturnRequested {
		t.Errorf("expected event type %s, got %s", events.EventTypeReturnRequested, evt.EventType)
	}
	if evt.Topic != events.TopicReturnRequested {
		t.Errorf("expected topic %s, got %s", events.TopicReturnRequested, evt.Topic)
	}

	var env events.EventEnvelope
	if err := json.Unmarshal(evt.Payload, &env); err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}
	var payload events.ReturnRequestedEvent
	if err := env.UnmarshalData(&payload); err != nil {
		t.Fatalf("failed to unmarshal ReturnRequested payload: %v", err)
	}
	if payload.ReturnID != res.ID {
		t.Errorf("expected payload ReturnID %s, got %s", res.ID, payload.ReturnID)
	}
	if payload.OrderItemID != itemID {
		t.Errorf("expected payload OrderItemID %s, got %s", itemID, payload.OrderItemID)
	}
	if payload.ProductID != productID {
		t.Errorf("expected payload ProductID %s, got %s", productID, payload.ProductID)
	}
}

func TestReturnService_RequestReturn_WithProductIDAsItemID(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "20000000-0000-0000-0000-000000000003"
	userID := "01a0e719-4b3e-72dd-93f0-13753c7ff053"
	actualOrderItemID := "30000000-0000-0000-0000-000000000004"
	productID := "ac3a9a16-f180-4704-a175-19ab7eec9b63"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, actualOrderItemID, productID, time.Now().UTC().Add(-2*24*time.Hour)),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{
			Id:           productID,
			IsReturnable: true,
		},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 7, nil)

	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: productID,
		UserID:      userID,
		Reason:      "Defective sound quality",
		Type:        "RETURN",
	}

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected success when passing product_id as item id, got error: %v", err)
	}

	if res.OrderItemID != actualOrderItemID {
		t.Errorf("expected canonical OrderItemID '%s', got '%s'", actualOrderItemID, res.OrderItemID)
	}
}

func TestReturnService_RequestReturn_OrderMustBeDelivered(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	nonDeliveredStatuses := []string{"CREATED", "CONFIRMED", "PROCESSING", "SHIPPED", "CANCELLED"}

	for _, st := range nonDeliveredStatuses {
		t.Run("Status_"+st, func(t *testing.T) {
			order := validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC())
			order.Status = st

			orderClient := &mockOrderClient{order: order}
			productClient := &mockProductClient{
				product: &productpb.Product{Id: productID, IsReturnable: true},
			}

			svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)
			req := &dto.RequestReturnDTO{
				OrderID:     orderID,
				OrderItemID: itemID,
				UserID:      userID,
				Reason:      "Doesn't fit",
			}

			_, err := svc.RequestReturn(ctx, req)
			if !errors.Is(err, service.ErrOrderNotDelivered) {
				t.Errorf("expected ErrOrderNotDelivered for status %s, got: %v", st, err)
			}
		})
	}
}

func TestReturnService_RequestReturn_ReturnWindowConfigurableAndExpiredRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	// Order delivered 15 days ago
	deliveredTime := time.Now().UTC().Add(-15 * 24 * time.Hour)
	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, deliveredTime),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	// 14-day return window: should be expired and rejected
	svc14 := service.NewReturnService(repo, orderClient, productClient, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Wrong size",
	}

	_, err := svc14.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrReturnWindowExpired) {
		t.Fatalf("expected ErrReturnWindowExpired for 14-day window, got: %v", err)
	}

	// Configurable: 30-day return window should accept the 15-day-old order
	svc30 := service.NewReturnService(repo, orderClient, productClient, 30, nil)
	res, err := svc30.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected success with 30-day window, got error: %v", err)
	}
	if res.ID == "" {
		t.Errorf("expected return ID to be generated")
	}
}

func TestReturnService_RequestReturn_NonReturnableProductRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-hygiene"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC().Add(-1*24*time.Hour)),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{
			Id:           productID,
			IsReturnable: false, // Non-returnable!
		},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Changed my mind",
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrProductNotReturnable) {
		t.Fatalf("expected ErrProductNotReturnable, got: %v", err)
	}
}

func TestReturnService_RequestReturn_ReasonMandatory(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC()),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)

	testCases := []struct {
		name   string
		reason string
	}{
		{"EmptyReason", ""},
		{"WhitespaceReason", "   \t \n  "},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			req := &dto.RequestReturnDTO{
				OrderID:     orderID,
				OrderItemID: itemID,
				UserID:      userID,
				Reason:      tc.reason,
			}
			_, err := svc.RequestReturn(ctx, req)
			if !errors.Is(err, service.ErrReturnReasonRequired) {
				t.Errorf("expected ErrReturnReasonRequired for reason %q, got: %v", tc.reason, err)
			}
		})
	}
}

func TestReturnService_RequestReturn_DuplicateRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC()),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)

	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "First return request",
	}

	// First request succeeds
	res1, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("first request failed: %v", err)
	}
	if res1 == nil {
		t.Fatalf("expected non-nil response")
	}

	// Second request for same item must be rejected as duplicate
	req2 := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Second return request for same item",
	}

	_, err = svc.RequestReturn(ctx, req2)
	if !errors.Is(err, service.ErrDuplicateReturnRequest) {
		t.Fatalf("expected ErrDuplicateReturnRequest, got: %v", err)
	}
}

func TestReturnService_RequestReturn_UserAssociation(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	orderOwnerID := "user-alice"
	callingUserID := "user-bob"
	itemID := "item-789"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, orderOwnerID, itemID, productID, time.Now().UTC()),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      callingUserID,
		Reason:      "Unauthorized caller trying to return Alice's item",
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrUnauthorizedReturnRequest) {
		t.Fatalf("expected ErrUnauthorizedReturnRequest, got: %v", err)
	}
}

func TestReturnService_RequestReturn_ItemNotFoundInOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, "actual-item-id", productID, time.Now().UTC()),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: "non-existent-item",
		UserID:      userID,
		Reason:      "Item not in order",
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrOrderItemNotFound) {
		t.Fatalf("expected ErrOrderItemNotFound, got: %v", err)
	}
}

func TestReturnService_RequestReturn_FallbackToItemIDs(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-123"
	userID := "user-456"
	itemID := "item-789"
	productID := "prod-101"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC()),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 14, nil)
	// Omitting OrderItemID, providing ItemIDs
	req := &dto.RequestReturnDTO{
		OrderID: orderID,
		UserID:  userID,
		Reason:  "Compatibility test",
		ItemIDs: []string{itemID},
	}

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected success with ItemIDs fallback, got: %v", err)
	}
	if res.OrderItemID != itemID {
		t.Errorf("expected OrderItemID %s, got %s", itemID, res.OrderItemID)
	}
}

func TestReturnService_RequestReturn_MultipleItemIDsRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	svc := service.NewReturnService(repo, nil, nil, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID: "order-123",
		UserID:  "user-456",
		Reason:  "Defective",
		ItemIDs: []string{"item-1", "item-2"},
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrMultipleItemsNotSupported) {
		t.Fatalf("expected ErrMultipleItemsNotSupported, got: %v", err)
	}
}

func TestReturnService_RequestReturn_ItemIDsMismatchRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	svc := service.NewReturnService(repo, nil, nil, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     "order-123",
		OrderItemID: "item-1",
		UserID:      "user-456",
		Reason:      "Defective",
		ItemIDs:     []string{"item-2"},
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrOrderItemMismatch) {
		t.Fatalf("expected ErrOrderItemMismatch, got: %v", err)
	}
}

func TestReturnService_GetReturn_AndUpdateReturnStatus(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	ret := &model.Return{
		ID:           "ret-1",
		OrderID:      "order-1",
		OrderItemID:  "item-1",
		UserID:       "user-1",
		Reason:       "Damaged",
		Status:       model.ReturnStatusRequested,
		RefundAmount: 25.0,
		RequestedAt:  time.Now().UTC(),
		CreatedAt:    time.Now().UTC(),
		UpdatedAt:    time.Now().UTC(),
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)

	svc := service.NewReturnService(repo, nil, nil, 14, nil)

	// GetReturn success
	got, err := svc.GetReturn(ctx, "ret-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "ret-1" {
		t.Errorf("expected ID ret-1, got %s", got.ID)
	}

	// GetReturn not found
	_, err = svc.GetReturn(ctx, "non-existent")
	if !errors.Is(err, repository.ErrReturnNotFound) {
		t.Errorf("expected ErrReturnNotFound, got: %v", err)
	}

	// UpdateReturnStatus success
	updated, err := svc.UpdateReturnStatus(ctx, "ret-1", model.ReturnStatusApproved)
	if err != nil {
		t.Fatalf("failed to update status: %v", err)
	}
	if updated.Status != model.ReturnStatusApproved {
		t.Errorf("expected status %s, got %s", model.ReturnStatusApproved, updated.Status)
	}

	// UpdateReturnStatus invalid status
	_, err = svc.UpdateReturnStatus(ctx, "ret-1", model.ReturnStatus("INVALID_STATUS"))
	if !errors.Is(err, service.ErrInvalidReturnStatus) {
		t.Errorf("expected ErrInvalidReturnStatus, got: %v", err)
	}
}

func TestReturnService_UpdateReturnStatus_AllowedAndInvalidTransitions(t *testing.T) {
	ctx := context.Background()

	t.Run("ValidAndInvalidFromRequested", func(t *testing.T) {
		repo := newMockReturnRepo()
		ret := &model.Return{
			ID:          "ret-req",
			OrderID:     "order-1",
			OrderItemID: "item-1",
			UserID:      "user-1",
			Status:      model.ReturnStatusRequested,
		}
		_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
		svc := service.NewReturnService(repo, nil, nil, 14, nil)

		// Invalid: jumping from REQUESTED directly to COMPLETED or REFUND_INITIATED
		_, err := svc.UpdateReturnStatus(ctx, "ret-req", model.ReturnStatusCompleted)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for REQUESTED -> COMPLETED, got: %v", err)
		}

		_, err = svc.UpdateReturnStatus(ctx, "ret-req", model.ReturnStatusRefundInitiated)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for REQUESTED -> REFUND_INITIATED, got: %v", err)
		}

		// Valid: REQUESTED -> APPROVED
		updated, err := svc.UpdateReturnStatus(ctx, "ret-req", model.ReturnStatusApproved)
		if err != nil {
			t.Fatalf("expected REQUESTED -> APPROVED to succeed, got: %v", err)
		}
		if updated.Status != model.ReturnStatusApproved {
			t.Errorf("expected status %s, got %s", model.ReturnStatusApproved, updated.Status)
		}
	})

	t.Run("TransitionsFromVerified", func(t *testing.T) {
		repo := newMockReturnRepo()
		ret := &model.Return{
			ID:          "ret-ver",
			OrderID:     "order-1",
			OrderItemID: "item-1",
			UserID:      "user-1",
			Status:      model.ReturnStatusVerified,
		}
		_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
		svc := service.NewReturnService(repo, nil, nil, 14, nil)

		// Invalid: backward transition VERIFIED -> RETURN_REQUESTED or APPROVED
		_, err := svc.UpdateReturnStatus(ctx, "ret-ver", model.ReturnStatusRequested)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for VERIFIED -> RETURN_REQUESTED, got: %v", err)
		}

		_, err = svc.UpdateReturnStatus(ctx, "ret-ver", model.ReturnStatusApproved)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for VERIFIED -> APPROVED, got: %v", err)
		}

		// Valid: VERIFIED -> REFUND_INITIATED
		updated, err := svc.UpdateReturnStatus(ctx, "ret-ver", model.ReturnStatusRefundInitiated)
		if err != nil {
			t.Fatalf("expected VERIFIED -> REFUND_INITIATED to succeed, got: %v", err)
		}
		if updated.Status != model.ReturnStatusRefundInitiated {
			t.Errorf("expected status %s, got %s", model.ReturnStatusRefundInitiated, updated.Status)
		}
	})

	t.Run("TransitionsFromRefundInitiated", func(t *testing.T) {
		repo := newMockReturnRepo()
		ret := &model.Return{
			ID:          "ret-ref",
			OrderID:     "order-1",
			OrderItemID: "item-1",
			UserID:      "user-1",
			Status:      model.ReturnStatusRefundInitiated,
		}
		_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
		svc := service.NewReturnService(repo, nil, nil, 14, nil)

		// Invalid: backward transition REFUND_INITIATED -> VERIFIED
		_, err := svc.UpdateReturnStatus(ctx, "ret-ref", model.ReturnStatusVerified)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for REFUND_INITIATED -> VERIFIED, got: %v", err)
		}

		// Valid: REFUND_INITIATED -> COMPLETED
		updated, err := svc.UpdateReturnStatus(ctx, "ret-ref", model.ReturnStatusCompleted)
		if err != nil {
			t.Fatalf("expected REFUND_INITIATED -> COMPLETED to succeed, got: %v", err)
		}
		if updated.Status != model.ReturnStatusCompleted {
			t.Errorf("expected status %s, got %s", model.ReturnStatusCompleted, updated.Status)
		}

		// Terminal: COMPLETED cannot transition to anything
		_, err = svc.UpdateReturnStatus(ctx, "ret-ref", model.ReturnStatusApproved)
		if !errors.Is(err, service.ErrInvalidStateTransition) {
			t.Errorf("expected ErrInvalidStateTransition for COMPLETED -> APPROVED, got: %v", err)
		}
	})
}

func TestReturnService_7DayEligibility_DefaultWindow(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-7day"
	userID := "user-7day"
	itemID := "item-7day"
	productID := "prod-7day"

	// 1. Order delivered 8 days ago -> should be rejected under default 7-day window
	delivered8DaysAgo := time.Now().UTC().Add(-8 * 24 * time.Hour)
	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, delivered8DaysAgo),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	// 0 specifies default window (which must be 7 days)
	svc := service.NewReturnService(repo, orderClient, productClient, 0, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Defective product",
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrReturnWindowExpired) {
		t.Fatalf("expected ErrReturnWindowExpired for 8 days old delivery under default 7-day window, got: %v", err)
	}

	// 2. Order delivered 6 days ago -> should be accepted
	delivered6DaysAgo := time.Now().UTC().Add(-6 * 24 * time.Hour)
	orderClient.order = validDeliveredOrder(orderID, userID, itemID, productID, delivered6DaysAgo)

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected success within 7 days, got: %v", err)
	}
	if res.ID == "" {
		t.Errorf("expected generated return ID")
	}
	if res.Type != model.ReturnTypeReturn {
		t.Errorf("expected default type RETURN, got: %s", res.Type)
	}
}

func TestReturnService_RequestReplacement_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-rep"
	userID := "user-rep"
	itemID := "item-rep"
	productID := "prod-rep"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC().Add(-2*24*time.Hour)),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 7, nil)
	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Wrong size, need replacement",
		Type:        "REPLACEMENT",
	}

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected successful replacement request, got: %v", err)
	}
	if res.Type != model.ReturnTypeReplacement {
		t.Errorf("expected Type REPLACEMENT, got: %s", res.Type)
	}
	if res.Status != model.ReturnStatusReplacementRequested {
		t.Errorf("expected Status REPLACEMENT_REQUESTED, got: %s", res.Status)
	}
}

func TestReturnService_VerifyProduct_Workflow(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	now := time.Now().UTC()

	ret := &model.Return{
		ID:           "ret-verify-1",
		OrderID:      "order-verify",
		OrderItemID:  "item-verify",
		UserID:       "user-verify",
		Reason:       "Damaged on arrival",
		Status:       model.ReturnStatusRequested,
		Type:         model.ReturnTypeReturn,
		RefundAmount: 150.0,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)

	svc := service.NewReturnService(repo, nil, nil, 7, nil)

	// 1. Optional proof images: verifying without proof images succeeds
	retWithoutImages := &model.Return{
		ID:           "ret-verify-no-img",
		OrderID:      "order-verify",
		OrderItemID:  "item-verify-no-img",
		UserID:       "user-verify",
		Reason:       "Wrong item",
		Status:       model.ReturnStatusRequested,
		Type:         model.ReturnTypeReturn,
		RefundAmount: 100.0,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_ = repo.CreateReturnWithOutbox(ctx, retWithoutImages, nil)

	noImgRet, noImgVerification, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    retWithoutImages.ID,
		VerifiedBy:  "delivery-partner-1",
		Condition:   "GOOD",
		IsApproved:  true,
		ProofImages: []string{},
	})
	if err != nil {
		t.Fatalf("expected verification without proof images to succeed, got: %v", err)
	}
	if noImgRet.Status != model.ReturnStatusVerified {
		t.Errorf("expected status VERIFIED, got: %s", noImgRet.Status)
	}
	if noImgVerification == nil || len(noImgVerification.ProofImages) != 0 {
		t.Errorf("expected 0 proof images, got: %v", noImgVerification)
	}

	// 2. Missing verifiedBy -> must fail
	_, _, err = svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    ret.ID,
		VerifiedBy:  "",
		Condition:   "GOOD",
		IsApproved:  true,
		ProofImages: []string{"https://storage.gocart.com/returns/img1.jpg"},
	})
	if !errors.Is(err, service.ErrVerifiedByRequired) {
		t.Fatalf("expected ErrVerifiedByRequired, got: %v", err)
	}

	// 3. Invalid condition -> must fail
	_, _, err = svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    ret.ID,
		VerifiedBy:  "delivery-partner-1",
		Condition:   "SUPERB_UNKNOWN",
		IsApproved:  true,
		ProofImages: []string{"https://storage.gocart.com/returns/img1.jpg"},
	})
	if !errors.Is(err, service.ErrInvalidVerificationCondition) {
		t.Fatalf("expected ErrInvalidVerificationCondition, got: %v", err)
	}

	// 4. Successful verification with ProofImages
	updatedRet, verification, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    ret.ID,
		VerifiedBy:  "delivery-partner-1",
		Condition:   "DAMAGED",
		IsApproved:  true,
		ProofImages: []string{"https://storage.gocart.com/returns/proof1.jpg", "https://storage.gocart.com/returns/proof2.jpg"},
		Notes:       "Item verified at customer doorstep. Packaging damaged as reported.",
	})
	if err != nil {
		t.Fatalf("unexpected error verifying product: %v", err)
	}
	if updatedRet.Status != model.ReturnStatusVerified {
		t.Errorf("expected status VERIFIED, got: %s", updatedRet.Status)
	}
	if verification == nil || len(verification.ProofImages) != 2 {
		t.Fatalf("expected 2 proof images, got: %v", verification)
	}
	if verification.Condition != model.ConditionDamaged {
		t.Errorf("expected condition DAMAGED, got: %s", verification.Condition)
	}

	// 5. Verifying again when status is VERIFIED should be rejected
	_, _, err = svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    ret.ID,
		VerifiedBy:  "delivery-partner-1",
		Condition:   "GOOD",
		IsApproved:  true,
		ProofImages: []string{"https://storage.gocart.com/returns/img2.jpg"},
	})
	if !errors.Is(err, service.ErrReturnNotVerifiable) {
		t.Fatalf("expected ErrReturnNotVerifiable for already verified return, got: %v", err)
	}
}

func TestReturnService_VerifyProduct_Rejection(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	now := time.Now().UTC()

	ret := &model.Return{
		ID:           "ret-reject-1",
		OrderID:      "order-reject",
		OrderItemID:  "item-reject",
		UserID:       "user-reject",
		Reason:       "Customer claimed defective",
		Status:       model.ReturnStatusRequested,
		Type:         model.ReturnTypeReturn,
		RefundAmount: 50.0,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)

	svc := service.NewReturnService(repo, nil, nil, 7, nil)

	// Delivery partner inspects and rejects (e.g. customer damaged product artificially)
	updatedRet, verification, err := svc.VerifyProduct(ctx, &dto.VerifyProductDTO{
		ReturnID:    ret.ID,
		VerifiedBy:  "delivery-partner-2",
		Condition:   "DAMAGED",
		IsApproved:  false,
		ProofImages: []string{"https://storage.gocart.com/returns/rejection_proof.jpg"},
		Notes:       "Item serial number does not match original product.",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if updatedRet.Status != model.ReturnStatusRejected {
		t.Errorf("expected status REJECTED, got: %s", updatedRet.Status)
	}
	if verification.IsApproved {
		t.Errorf("expected IsApproved to be false")
	}
}

