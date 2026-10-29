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
	exchanges    map[string]*model.Exchange
	outboxEvents []*outbox.Event
}

func newMockReturnRepo() *mockReturnRepo {
	return &mockReturnRepo{
		returns:      make(map[string]*model.Return),
		byOrderItem:  make(map[string]*model.Return),
		exchanges:    make(map[string]*model.Exchange),
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
	return m.UpdateReturnStatusWithOutbox(ctx, id, status, "")
}

func (m *mockReturnRepo) UpdateReturnStatusWithOutbox(ctx context.Context, id string, status model.ReturnStatus, rejectionReason string, outboxEvents ...*outbox.Event) (*model.Return, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ret, ok := m.returns[id]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	now := time.Now().UTC()
	ret.Status = status
	ret.UpdatedAt = now
	if status == model.ReturnStatusApproved {
		ret.ApprovedAt = &now
	}
	if status == model.ReturnStatusCompleted {
		ret.CompletedAt = &now
	}
	if rejectionReason != "" {
		ret.RejectionReason = rejectionReason
	}
	if len(outboxEvents) > 0 {
		m.outboxEvents = append(m.outboxEvents, outboxEvents...)
	}
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

func (m *mockReturnRepo) CreateExchangeWithOutbox(ctx context.Context, exc *model.Exchange, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	cp := *exc
	cp.Answers = answers
	if m.exchanges == nil {
		m.exchanges = make(map[string]*model.Exchange)
	}
	m.exchanges[exc.ID] = &cp
	if outboxEvent != nil {
		m.outboxEvents = append(m.outboxEvents, outboxEvent)
	}
	return nil
}

func (m *mockReturnRepo) GetExchangeByID(ctx context.Context, id string) (*model.Exchange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[id]
	if !ok {
		return nil, repository.ErrExchangeNotFound
	}
	cp := *exc
	return &cp, nil
}

func (m *mockReturnRepo) GetExchangeByOrderID(ctx context.Context, orderID string) (*model.Exchange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, exc := range m.exchanges {
		if exc.OrderID == orderID {
			cp := *exc
			return &cp, nil
		}
	}
	return nil, repository.ErrExchangeNotFound
}

func (m *mockReturnRepo) CreateExchangeVerificationWithOutbox(ctx context.Context, v *model.ExchangeVerification, newStatus model.ExchangeStatus, finalDiscount float64, outboxEvents ...*outbox.Event) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	exc, ok := m.exchanges[v.ExchangeID]
	if !ok {
		return repository.ErrExchangeNotFound
	}
	exc.Status = newStatus
	exc.FinalDiscount = finalDiscount
	exc.Verification = v
	exc.UpdatedAt = time.Now().UTC()
	if len(outboxEvents) > 0 {
		m.outboxEvents = append(m.outboxEvents, outboxEvents...)
	}
	return nil
}

func (m *mockReturnRepo) GetExchangeVerification(ctx context.Context, exchangeID string) (*model.ExchangeVerification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[exchangeID]
	if !ok || exc.Verification == nil {
		return nil, nil
	}
	cp := *exc.Verification
	return &cp, nil
}

func (m *mockReturnRepo) GetExchangeQuestionAnswers(ctx context.Context, exchangeID string) ([]*model.ExchangeQuestionAnswer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[exchangeID]
	if !ok {
		return nil, nil
	}
	return exc.Answers, nil
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

func TestReturnService_RequestReturn_MissingOrderItemIDRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	svc := service.NewReturnService(repo, nil, nil, 14, nil)
	req := &dto.RequestReturnDTO{
		OrderID: "order-123",
		UserID:  "user-456",
		Reason:  "Defective",
	}

	_, err := svc.RequestReturn(ctx, req)
	if !errors.Is(err, service.ErrOrderItemRequired) {
		t.Fatalf("expected ErrOrderItemRequired, got: %v", err)
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

func TestReturnService_Lifecycle_FullProgression(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	now := time.Now().UTC()

	ret := &model.Return{
		ID:           "ret-lifecycle-1",
		OrderID:      "order-1",
		OrderItemID:  "item-1",
		UserID:       "cust-123",
		Reason:       "Wrong size",
		Status:       model.ReturnStatusRequested,
		Type:         model.ReturnTypeReturn,
		RefundAmount: 50.0,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)

	svc := service.NewReturnService(repo, nil, nil, 14, nil)
	adminID := "admin-999"

	// 1. REQUESTED -> UNDER_REVIEW
	updated, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusUnderReview, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to UNDER_REVIEW, got err: %v", err)
	}
	if updated.Status != model.ReturnStatusUnderReview {
		t.Errorf("expected status UNDER_REVIEW, got: %s", updated.Status)
	}

	// 2. UNDER_REVIEW -> APPROVED (via ApproveReturn)
	approved, err := svc.ApproveReturn(ctx, ret.ID, adminID)
	if err != nil {
		t.Fatalf("expected approval to succeed, got err: %v", err)
	}
	if approved.Status != model.ReturnStatusApproved {
		t.Errorf("expected status APPROVED, got: %s", approved.Status)
	}

	// 3. APPROVED -> PICKUP_SCHEDULED
	pickupScheduled, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusPickupScheduled, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to PICKUP_SCHEDULED, got err: %v", err)
	}
	if pickupScheduled.Status != model.ReturnStatusPickupScheduled {
		t.Errorf("expected status PICKUP_SCHEDULED, got: %s", pickupScheduled.Status)
	}

	// 4. PICKUP_SCHEDULED -> PICKED_UP
	pickedUp, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusPickedUp, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to PICKED_UP, got err: %v", err)
	}
	if pickedUp.Status != model.ReturnStatusPickedUp {
		t.Errorf("expected status PICKED_UP, got: %s", pickedUp.Status)
	}

	// 5. PICKED_UP -> RECEIVED
	received, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusReceived, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to RECEIVED, got err: %v", err)
	}
	if received.Status != model.ReturnStatusReceived {
		t.Errorf("expected status RECEIVED, got: %s", received.Status)
	}

	// 6. RECEIVED -> REFUND_INITIATED
	refundInit, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusRefundInitiated, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to REFUND_INITIATED, got err: %v", err)
	}
	if refundInit.Status != model.ReturnStatusRefundInitiated {
		t.Errorf("expected status REFUND_INITIATED, got: %s", refundInit.Status)
	}

	// 7. REFUND_INITIATED -> COMPLETED
	completed, err := svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusCompleted, "", adminID)
	if err != nil {
		t.Fatalf("expected transition to COMPLETED, got err: %v", err)
	}
	if completed.Status != model.ReturnStatusCompleted {
		t.Errorf("expected status COMPLETED, got: %s", completed.Status)
	}
}

func TestReturnService_ApproveAndReject_OutboxEvents(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	adminID := "admin-reviewer"
	custID := "customer-1"

	t.Run("ApproveReturn_OutboxEvents", func(t *testing.T) {
		repo := newMockReturnRepo()
		ret := &model.Return{
			ID:           "ret-app-evt",
			OrderID:      "order-app",
			OrderItemID:  "item-app",
			UserID:       custID,
			Reason:       "Defective",
			Status:       model.ReturnStatusRequested,
			Type:         model.ReturnTypeReturn,
			RefundAmount: 75.0,
			RequestedAt:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
		svc := service.NewReturnService(repo, nil, nil, 14, nil)

		approved, err := svc.ApproveReturn(ctx, ret.ID, adminID)
		if err != nil {
			t.Fatalf("unexpected error approving return: %v", err)
		}
		if approved.Status != model.ReturnStatusApproved {
			t.Errorf("expected status APPROVED, got: %s", approved.Status)
		}

		// Must have 2 outbox events: ReturnApproved and ReturnStatusUpdated
		if len(repo.outboxEvents) != 2 {
			t.Fatalf("expected 2 outbox events, got: %d", len(repo.outboxEvents))
		}

		approvedEvt := repo.outboxEvents[0]
		if approvedEvt.EventType != events.EventTypeReturnApproved {
			t.Errorf("expected event type %s, got: %s", events.EventTypeReturnApproved, approvedEvt.EventType)
		}
		if approvedEvt.Topic != events.TopicReturnApproved {
			t.Errorf("expected topic %s, got: %s", events.TopicReturnApproved, approvedEvt.Topic)
		}

		var env events.EventEnvelope
		if err := json.Unmarshal(approvedEvt.Payload, &env); err != nil {
			t.Fatalf("failed to unmarshal approved envelope: %v", err)
		}
		var payload events.ReturnApprovedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ReturnApprovedEvent payload: %v", err)
		}
		if payload.ReturnID != ret.ID {
			t.Errorf("expected ReturnID %s, got: %s", ret.ID, payload.ReturnID)
		}
		if payload.ApprovedBy != adminID {
			t.Errorf("expected ApprovedBy %s, got: %s", adminID, payload.ApprovedBy)
		}
		if payload.UserID != custID {
			t.Errorf("expected UserID %s, got: %s", custID, payload.UserID)
		}

		statusUpdatedEvt := repo.outboxEvents[1]
		if statusUpdatedEvt.EventType != events.EventTypeReturnStatusUpdated {
			t.Errorf("expected event type %s, got: %s", events.EventTypeReturnStatusUpdated, statusUpdatedEvt.EventType)
		}
	})

	t.Run("RejectReturn_OutboxEvents", func(t *testing.T) {
		repo := newMockReturnRepo()
		ret := &model.Return{
			ID:           "ret-rej-evt",
			OrderID:      "order-rej",
			OrderItemID:  "item-rej",
			UserID:       custID,
			Reason:       "Defective",
			Status:       model.ReturnStatusRequested,
			Type:         model.ReturnTypeReturn,
			RefundAmount: 75.0,
			RequestedAt:  now,
			CreatedAt:    now,
			UpdatedAt:    now,
		}
		_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
		svc := service.NewReturnService(repo, nil, nil, 14, nil)

		rejectionReason := "Policy violation: product seal is broken"
		rejected, err := svc.RejectReturn(ctx, ret.ID, rejectionReason, adminID)
		if err != nil {
			t.Fatalf("unexpected error rejecting return: %v", err)
		}
		if rejected.Status != model.ReturnStatusRejected {
			t.Errorf("expected status REJECTED, got: %s", rejected.Status)
		}
		if rejected.RejectionReason != rejectionReason {
			t.Errorf("expected rejection reason %q, got: %q", rejectionReason, rejected.RejectionReason)
		}

		// Must have 2 outbox events: ReturnRejected and ReturnStatusUpdated
		if len(repo.outboxEvents) != 2 {
			t.Fatalf("expected 2 outbox events, got: %d", len(repo.outboxEvents))
		}

		rejectedEvt := repo.outboxEvents[0]
		if rejectedEvt.EventType != events.EventTypeReturnRejected {
			t.Errorf("expected event type %s, got: %s", events.EventTypeReturnRejected, rejectedEvt.EventType)
		}
		if rejectedEvt.Topic != events.TopicReturnRejected {
			t.Errorf("expected topic %s, got: %s", events.TopicReturnRejected, rejectedEvt.Topic)
		}

		var env events.EventEnvelope
		if err := json.Unmarshal(rejectedEvt.Payload, &env); err != nil {
			t.Fatalf("failed to unmarshal rejected envelope: %v", err)
		}
		var payload events.ReturnRejectedEvent
		if err := env.UnmarshalData(&payload); err != nil {
			t.Fatalf("failed to unmarshal ReturnRejectedEvent payload: %v", err)
		}
		if payload.ReturnID != ret.ID {
			t.Errorf("expected ReturnID %s, got: %s", ret.ID, payload.ReturnID)
		}
		if payload.RejectedBy != adminID {
			t.Errorf("expected RejectedBy %s, got: %s", adminID, payload.RejectedBy)
		}
		if payload.RejectionReason != rejectionReason {
			t.Errorf("expected RejectionReason %q, got: %q", rejectionReason, payload.RejectionReason)
		}
	})
}

func TestReturnService_CustomerCannotReviewOwnReturn(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	customerID := "customer-123"

	ret := &model.Return{
		ID:          "ret-customer-guard",
		OrderID:     "order-1",
		OrderItemID: "item-1",
		UserID:      customerID,
		Status:      model.ReturnStatusRequested,
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
	svc := service.NewReturnService(repo, nil, nil, 14, nil)

	// Customer tries to approve their own return
	_, err := svc.ApproveReturn(ctx, ret.ID, customerID)
	if !errors.Is(err, service.ErrCustomerCannotReviewOwnReturn) {
		t.Errorf("expected ErrCustomerCannotReviewOwnReturn, got: %v", err)
	}

	// Customer tries to reject their own return
	_, err = svc.RejectReturn(ctx, ret.ID, "Some reason", customerID)
	if !errors.Is(err, service.ErrCustomerCannotReviewOwnReturn) {
		t.Errorf("expected ErrCustomerCannotReviewOwnReturn, got: %v", err)
	}

	// Customer tries via UpdateReturnStatus
	_, err = svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusApproved, "", customerID)
	if !errors.Is(err, service.ErrCustomerCannotReviewOwnReturn) {
		t.Errorf("expected ErrCustomerCannotReviewOwnReturn, got: %v", err)
	}

	_, err = svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusRejected, "Some reason", customerID)
	if !errors.Is(err, service.ErrCustomerCannotReviewOwnReturn) {
		t.Errorf("expected ErrCustomerCannotReviewOwnReturn, got: %v", err)
	}
}

func TestReturnService_RejectReturn_ReasonValidation(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	adminID := "admin-reviewer"

	ret := &model.Return{
		ID:          "ret-reject-val",
		OrderID:     "order-1",
		OrderItemID: "item-1",
		UserID:      "customer-456",
		Status:      model.ReturnStatusRequested,
	}
	_ = repo.CreateReturnWithOutbox(ctx, ret, nil)
	svc := service.NewReturnService(repo, nil, nil, 14, nil)

	// Empty reason
	_, err := svc.RejectReturn(ctx, ret.ID, "", adminID)
	if !errors.Is(err, service.ErrRejectionReasonRequired) {
		t.Errorf("expected ErrRejectionReasonRequired for empty reason, got: %v", err)
	}

	// Whitespace reason
	_, err = svc.RejectReturn(ctx, ret.ID, "   \t \n ", adminID)
	if !errors.Is(err, service.ErrRejectionReasonRequired) {
		t.Errorf("expected ErrRejectionReasonRequired for whitespace reason, got: %v", err)
	}

	// Via UpdateReturnStatus without reason
	_, err = svc.UpdateReturnStatus(ctx, ret.ID, model.ReturnStatusRejected, "", adminID)
	if !errors.Is(err, service.ErrRejectionReasonRequired) {
		t.Errorf("expected ErrRejectionReasonRequired via UpdateReturnStatus, got: %v", err)
	}
}

func TestReturnService_Exchange_Lifecycle(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	orderID := "order-exch"
	userID := "user-exch"
	itemID := "item-exch"
	productID := "prod-exch"

	orderClient := &mockOrderClient{
		order: validDeliveredOrder(orderID, userID, itemID, productID, time.Now().UTC().Add(-2*24*time.Hour)),
	}
	productClient := &mockProductClient{
		product: &productpb.Product{Id: productID, IsReturnable: true},
	}

	svc := service.NewReturnService(repo, orderClient, productClient, 7, nil)
	adminID := "admin-1"

	req := &dto.RequestReturnDTO{
		OrderID:     orderID,
		OrderItemID: itemID,
		UserID:      userID,
		Reason:      "Wrong color, need exchange",
		Type:        "EXCHANGE",
	}

	res, err := svc.RequestReturn(ctx, req)
	if err != nil {
		t.Fatalf("expected successful exchange request, got: %v", err)
	}
	if res.Type != model.ReturnTypeExchange {
		t.Errorf("expected type EXCHANGE, got: %s", res.Type)
	}
	if res.Status != model.ReturnStatusExchangeRequested {
		t.Errorf("expected status EXCHANGE_REQUESTED, got: %s", res.Status)
	}

	// Transition: EXCHANGE_REQUESTED -> UNDER_REVIEW
	underReview, err := svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusUnderReview, "", adminID)
	if err != nil {
		t.Fatalf("expected EXCHANGE_REQUESTED -> UNDER_REVIEW, got err: %v", err)
	}
	if underReview.Status != model.ReturnStatusUnderReview {
		t.Errorf("expected status UNDER_REVIEW, got: %s", underReview.Status)
	}

	// UNDER_REVIEW -> APPROVED
	approved, err := svc.ApproveReturn(ctx, res.ID, adminID)
	if err != nil {
		t.Fatalf("expected approval of exchange, got err: %v", err)
	}
	if approved.Status != model.ReturnStatusApproved {
		t.Errorf("expected status APPROVED, got: %s", approved.Status)
	}

	// APPROVED -> PICKUP_SCHEDULED -> PICKED_UP -> RECEIVED -> EXCHANGE_INITIATED -> COMPLETED
	_, err = svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusPickupScheduled, "", adminID)
	if err != nil {
		t.Fatalf("failed pickup scheduled: %v", err)
	}
	_, err = svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusPickedUp, "", adminID)
	if err != nil {
		t.Fatalf("failed picked up: %v", err)
	}
	_, err = svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusReceived, "", adminID)
	if err != nil {
		t.Fatalf("failed received: %v", err)
	}
	exchInit, err := svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusExchangeInitiated, "", adminID)
	if err != nil {
		t.Fatalf("failed exchange initiated: %v", err)
	}
	if exchInit.Status != model.ReturnStatusExchangeInitiated {
		t.Errorf("expected EXCHANGE_INITIATED, got: %s", exchInit.Status)
	}
	completed, err := svc.UpdateReturnStatus(ctx, res.ID, model.ReturnStatusCompleted, "", adminID)
	if err != nil {
		t.Fatalf("failed completed: %v", err)
	}
	if completed.Status != model.ReturnStatusCompleted {
		t.Errorf("expected COMPLETED, got: %s", completed.Status)
	}
}

func TestReturnService_CreateExchange_SuccessAndCalculation(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	prodClient := &mockProductClient{
		product: &productpb.Product{
			Id:    "new-prod-999",
			Price: 799.0,
		},
	}
	svc := service.NewReturnService(repo, nil, prodClient, 7)

	req := &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		OrderItemID:       "item-456",
		UserID:            "user-789",
		NewProductID:      "new-prod-999",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		OldVariant:        "128GB",
		SerialOrIMEI:      "354829104829104",
		BaseExchangeValue: 300.0,
		Answers: []*dto.ExchangeAnswerDTO{
			{QuestionKey: "SCREEN_CONDITION", AnswerValue: "MINOR_SCRATCHES", DeductionAmount: 30.0},
			{QuestionKey: "BATTERY_HEALTH", AnswerValue: "NORMAL", DeductionAmount: 0.0},
			{QuestionKey: "BODY_CONDITION", AnswerValue: "DENTS", DeductionAmount: 20.0},
		},
	}

	exc, err := svc.CreateExchange(ctx, req)
	if err != nil {
		t.Fatalf("expected create exchange success, got err: %v", err)
	}
	if exc.BaseExchangeValue != 300.0 {
		t.Errorf("expected base value 300, got: %f", exc.BaseExchangeValue)
	}
	// 300 - 30 - 20 = 250
	if exc.EstimatedDiscount != 250.0 {
		t.Errorf("expected estimated discount 250, got: %f", exc.EstimatedDiscount)
	}
	// Price difference: 799 - 250 = 549
	if exc.PriceDifference != 549.0 {
		t.Errorf("expected price diff 549, got: %f", exc.PriceDifference)
	}
	if exc.Status != model.ExchangeStatusOfferCreated {
		t.Errorf("expected OFFER_CREATED, got: %s", exc.Status)
	}
	if len(exc.Answers) != 3 {
		t.Errorf("expected 3 answers, got: %d", len(exc.Answers))
	}
}

func TestReturnService_CreateExchange_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	// Missing user
	_, err := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:      "order-1",
		NewProductID: "prod-1",
		OldBrand:     "Brand",
		OldModel:     "Model",
	})
	if !errors.Is(err, service.ErrUserIDRequired) {
		t.Errorf("expected ErrUserIDRequired, got: %v", err)
	}

	// Missing order
	_, err = svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		UserID:       "user-1",
		NewProductID: "prod-1",
		OldBrand:     "Brand",
		OldModel:     "Model",
	})
	if !errors.Is(err, service.ErrOrderIDRequired) {
		t.Errorf("expected ErrOrderIDRequired, got: %v", err)
	}

	// Missing new product
	_, err = svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		UserID:   "user-1",
		OrderID:  "order-1",
		OldBrand: "Brand",
		OldModel: "Model",
	})
	if !errors.Is(err, service.ErrExchangeNewProductRequired) {
		t.Errorf("expected ErrExchangeNewProductRequired, got: %v", err)
	}

	// Missing brand/model
	_, err = svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		UserID:       "user-1",
		OrderID:      "order-1",
		NewProductID: "prod-1",
	})
	if !errors.Is(err, service.ErrExchangeBrandModelRequired) {
		t.Errorf("expected ErrExchangeBrandModelRequired, got: %v", err)
	}

	// Order client configured: order not found
	svcWithOrder := service.NewReturnService(repo, &mockOrderClient{order: nil}, nil, 7)
	_, err = svcWithOrder.CreateExchange(ctx, &dto.CreateExchangeDTO{
		UserID:       "user-1",
		OrderID:      "order-1",
		NewProductID: "prod-1",
		OldBrand:     "Brand",
		OldModel:     "Model",
	})
	if !errors.Is(err, service.ErrOrderNotFound) {
		t.Errorf("expected ErrOrderNotFound, got: %v", err)
	}

	// Order client configured: order belongs to another customer
	svcWithOtherUser := service.NewReturnService(repo, &mockOrderClient{
		order: &orderpb.Order{
			Id:     "order-1",
			UserId: "user-different",
		},
	}, nil, 7)
	_, err = svcWithOtherUser.CreateExchange(ctx, &dto.CreateExchangeDTO{
		UserID:       "user-1",
		OrderID:      "order-1",
		NewProductID: "prod-1",
		OldBrand:     "Brand",
		OldModel:     "Model",
	})
	if !errors.Is(err, service.ErrUnauthorizedReturnRequest) {
		t.Errorf("expected ErrUnauthorizedReturnRequest, got: %v", err)
	}
}

func TestReturnService_VerifyExchangeProduct_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, err := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Samsung",
		OldModel:          "Galaxy S21",
		BaseExchangeValue: 200.0,
	})
	if err != nil {
		t.Fatalf("failed create exchange: %v", err)
	}

	// Verify product approved
	adjDiscount := 200.0
	verifiedExc, v, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:       exc.ID,
		VerifiedBy:       "delivery-partner-1",
		ConditionRating:  "GOOD",
		IsApproved:       true,
		ScreenIntact:     true,
		BodyIntact:       true,
		Functional:       true,
		SerialMatches:    true,
		AdjustedDiscount: &adjDiscount,
		ProofImages:      []string{"https://img.example.com/proof1.jpg"},
		Notes:            "Device in good condition",
	})
	if err != nil {
		t.Fatalf("expected verify success, got err: %v", err)
	}
	if verifiedExc.Status != model.ExchangeStatusVerified {
		t.Errorf("expected VERIFIED, got: %s", verifiedExc.Status)
	}
	if v == nil || !v.IsApproved {
		t.Errorf("expected verification approved")
	}
	if len(v.ProofImages) != 1 {
		t.Errorf("expected 1 proof image, got: %d", len(v.ProofImages))
	}
}

func TestReturnService_VerifyExchangeProduct_ZeroAdjustedDiscountAllowed(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, err := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 500.0,
	})
	if err != nil {
		t.Fatalf("failed create exchange: %v", err)
	}
	if exc.EstimatedDiscount <= 0 {
		t.Fatalf("expected positive estimated discount, got %f", exc.EstimatedDiscount)
	}

	zeroDiscount := 0.0
	verifiedExc, v, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:       exc.ID,
		VerifiedBy:       "delivery-partner-1",
		ConditionRating:  "DAMAGED",
		IsApproved:       true,
		ScreenIntact:     true,
		BodyIntact:       true,
		Functional:       true,
		SerialMatches:    true,
		AdjustedDiscount: &zeroDiscount,
		ProofImages:      []string{"https://img.example.com/proof.jpg"},
	})
	if err != nil {
		t.Fatalf("expected verify success, got err: %v", err)
	}
	if verifiedExc.Status != model.ExchangeStatusRevaluated {
		t.Errorf("expected REVALUATED, got: %s", verifiedExc.Status)
	}
	if verifiedExc.FinalDiscount != 0.0 {
		t.Errorf("expected FinalDiscount 0.0, got: %f", verifiedExc.FinalDiscount)
	}
	if v.AdjustedDiscount != 0.0 {
		t.Errorf("expected verification AdjustedDiscount 0.0, got: %f", v.AdjustedDiscount)
	}
}

func TestReturnService_VerifyExchangeProduct_OmittedDiscountUsesEstimated(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, err := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 500.0,
	})
	if err != nil {
		t.Fatalf("failed create exchange: %v", err)
	}

	verifiedExc, v, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:       exc.ID,
		VerifiedBy:       "delivery-partner-1",
		ConditionRating:  "GOOD",
		IsApproved:       true,
		ScreenIntact:     true,
		BodyIntact:       true,
		Functional:       true,
		SerialMatches:    true,
		AdjustedDiscount: nil,
		ProofImages:      []string{"https://img.example.com/proof.jpg"},
	})
	if err != nil {
		t.Fatalf("expected verify success, got err: %v", err)
	}
	if verifiedExc.Status != model.ExchangeStatusVerified {
		t.Errorf("expected VERIFIED, got: %s", verifiedExc.Status)
	}
	if verifiedExc.FinalDiscount != exc.EstimatedDiscount {
		t.Errorf("expected FinalDiscount %f, got: %f", exc.EstimatedDiscount, verifiedExc.FinalDiscount)
	}
	if v.AdjustedDiscount != exc.EstimatedDiscount {
		t.Errorf("expected verification AdjustedDiscount %f, got: %f", exc.EstimatedDiscount, v.AdjustedDiscount)
	}
}

func TestReturnService_VerifyExchangeProduct_Disapproved(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, _ := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Samsung",
		OldModel:          "Galaxy S21",
		BaseExchangeValue: 200.0,
	})

	// Verify product disapproved
	verifiedExc, v, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:      exc.ID,
		VerifiedBy:      "delivery-partner-1",
		ConditionRating: "DAMAGED",
		IsApproved:      false,
		ScreenIntact:    false,
		Functional:      false,
		SerialMatches:   true,
		Notes:           "Screen completely broken",
	})
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if verifiedExc.Status != model.ExchangeStatusCancelled {
		t.Errorf("expected CANCELLED, got: %s", verifiedExc.Status)
	}
	if verifiedExc.FinalDiscount != 0 {
		t.Errorf("expected 0 final discount, got: %f", verifiedExc.FinalDiscount)
	}
	if v.IsApproved {
		t.Errorf("expected not approved")
	}
}

func TestReturnService_VerifyExchangeProduct_NonFunctionalCannotBeApproved(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, _ := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Samsung",
		OldModel:          "Galaxy S21",
		BaseExchangeValue: 200.0,
	})

	// Attempting to approve non-functional device
	_, _, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:      exc.ID,
		VerifiedBy:      "delivery-partner-1",
		ConditionRating: "POOR",
		IsApproved:      true,
		Functional:      false,
		SerialMatches:   true,
	})
	if !errors.Is(err, service.ErrExchangeFunctionalRequired) {
		t.Errorf("expected ErrExchangeFunctionalRequired, got: %v", err)
	}
}

func TestReturnService_VerifyExchangeProduct_InvalidConditionRating(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, nil, nil, 7)

	exc, _ := svc.CreateExchange(ctx, &dto.CreateExchangeDTO{
		OrderID:           "order-123",
		UserID:            "user-789",
		NewProductID:      "prod-999",
		OldBrand:          "Samsung",
		OldModel:          "Galaxy S21",
		BaseExchangeValue: 200.0,
	})

	_, _, err := svc.VerifyExchangeProduct(ctx, &dto.VerifyExchangeProductDTO{
		ExchangeID:      exc.ID,
		VerifiedBy:      "delivery-partner-1",
		ConditionRating: "INVALID_RATING",
		IsApproved:      true,
		Functional:      true,
		SerialMatches:   true,
	})
	if !errors.Is(err, service.ErrInvalidExchangeConditionRating) {
		t.Errorf("expected ErrInvalidExchangeConditionRating, got: %v", err)
	}
}

