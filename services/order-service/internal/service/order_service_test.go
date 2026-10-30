package service_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// Mock Order Repository
type mockOrderRepo struct {
	mu              sync.RWMutex
	orders          map[string]*model.Order
	history         map[string][]*model.OrderStatusHistory
	outboxEnvelopes []*events.EventEnvelope
	outboxTopic     string
}

func newMockOrderRepo() *mockOrderRepo {
	return &mockOrderRepo{
		orders:          make(map[string]*model.Order),
		history:         make(map[string][]*model.OrderStatusHistory),
		outboxEnvelopes: make([]*events.EventEnvelope, 0),
	}
}

func (m *mockOrderRepo) CreateOrdersWithOutbox(
	ctx context.Context,
	orders []*model.Order,
	envelopes []*events.EventEnvelope,
	topic string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, o := range orders {
		cp := *o
		cp.Items = make([]model.OrderItem, len(o.Items))
		copy(cp.Items, o.Items)
		m.orders[o.ID] = &cp

		m.history[o.ID] = append(m.history[o.ID], &model.OrderStatusHistory{
			ID:        uuid.New().String(),
			OrderID:   o.ID,
			ToStatus:  o.Status,
			Reason:    "Order created",
			ChangedBy: o.UserID,
			CreatedAt: o.CreatedAt,
		})
	}
	m.outboxEnvelopes = append(m.outboxEnvelopes, envelopes...)
	m.outboxTopic = topic
	return nil
}

func (m *mockOrderRepo) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orders[id]
	if !ok {
		return nil, repository.ErrOrderNotFound
	}
	cp := *o
	cp.Items = make([]model.OrderItem, len(o.Items))
	copy(cp.Items, o.Items)
	return &cp, nil
}

func (m *mockOrderRepo) GetOrdersByIdempotencyKey(ctx context.Context, idempotencyKey string) ([]*model.Order, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var matchingPrimary *model.Order
	for _, o := range m.orders {
		if o.IdempotencyKey != nil && *o.IdempotencyKey == idempotencyKey {
			matchingPrimary = o
			break
		}
	}
	if matchingPrimary == nil {
		return nil, repository.ErrOrderNotFound
	}

	res := make([]*model.Order, 0)
	if matchingPrimary.ParentOrderID != nil && *matchingPrimary.ParentOrderID != "" {
		for _, o := range m.orders {
			if o.ParentOrderID != nil && *o.ParentOrderID == *matchingPrimary.ParentOrderID {
				cp := *o
				cp.Items = make([]model.OrderItem, len(o.Items))
				copy(cp.Items, o.Items)
				res = append(res, &cp)
			}
		}
		if len(res) > 0 {
			return res, nil
		}
	}

	cp := *matchingPrimary
	cp.Items = make([]model.OrderItem, len(matchingPrimary.Items))
	copy(cp.Items, matchingPrimary.Items)
	return []*model.Order{&cp}, nil
}

func (m *mockOrderRepo) ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Order, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	res := make([]*model.Order, 0)
	for _, o := range m.orders {
		if o.UserID == userID {
			cp := *o
			cp.Items = make([]model.OrderItem, len(o.Items))
			copy(cp.Items, o.Items)
			res = append(res, &cp)
		}
	}
	total := len(res)
	if offset > total {
		return []*model.Order{}, total, nil
	}
	end := offset + limit
	if end > total {
		end = total
	}
	return res[offset:end], total, nil
}

func (m *mockOrderRepo) UpdateOrderStatus(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return nil, repository.ErrOrderNotFound
	}
	prev := o.Status
	o.Status = orderStatus
	o.UpdatedAt = time.Now().UTC()

	rText := ""
	if len(reason) > 0 {
		rText = reason[0]
	}

	m.history[id] = append(m.history[id], &model.OrderStatusHistory{
		ID:         uuid.New().String(),
		OrderID:    id,
		FromStatus: &prev,
		ToStatus:   orderStatus,
		Reason:     rText,
		CreatedAt:  o.UpdatedAt,
	})

	cp := *o
	cp.Items = make([]model.OrderItem, len(o.Items))
	copy(cp.Items, o.Items)
	return &cp, nil
}

func (m *mockOrderRepo) UpdateOrderStatusWithOutbox(ctx context.Context, id string, orderStatus model.OrderStatus, envelope *events.EventEnvelope, topic string, reason ...string) (*model.Order, error) {
	if envelope != nil {
		m.mu.Lock()
		m.outboxEnvelopes = append(m.outboxEnvelopes, envelope)
		m.outboxTopic = topic
		m.mu.Unlock()
	}
	return m.UpdateOrderStatus(ctx, id, orderStatus, reason...)
}

func (m *mockOrderRepo) CancelOrder(ctx context.Context, id string, envelope *events.EventEnvelope, topic string, reason ...string) (*model.Order, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	o, ok := m.orders[id]
	if !ok {
		return nil, repository.ErrOrderNotFound
	}
	prev := o.Status
	o.Status = model.OrderStatusCancelled
	o.UpdatedAt = time.Now().UTC()
	if envelope != nil {
		m.outboxEnvelopes = append(m.outboxEnvelopes, envelope)
		m.outboxTopic = topic
	}

	rText := ""
	if len(reason) > 0 {
		rText = reason[0]
	}

	m.history[id] = append(m.history[id], &model.OrderStatusHistory{
		ID:         uuid.New().String(),
		OrderID:    id,
		FromStatus: &prev,
		ToStatus:   model.OrderStatusCancelled,
		Reason:     rText,
		CreatedAt:  o.UpdatedAt,
	})

	cp := *o
	cp.Items = make([]model.OrderItem, len(o.Items))
	copy(cp.Items, o.Items)
	return &cp, nil
}

func (m *mockOrderRepo) GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	list, ok := m.history[orderID]
	if !ok {
		return []*model.OrderStatusHistory{}, nil
	}
	cp := make([]*model.OrderStatusHistory, len(list))
	copy(cp, list)
	return cp, nil
}

func (m *mockOrderRepo) CreateOrderWithOutbox(
	ctx context.Context,
	orderID string,
	insertOrder func(ctx context.Context, tx pgx.Tx) error,
	envelope *events.EventEnvelope,
	topic string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outboxEnvelopes = append(m.outboxEnvelopes, envelope)
	m.outboxTopic = topic
	return nil
}

// Mock Cart Client
type mockCartClient struct {
	mu         sync.RWMutex
	carts      map[string]*cartpb.Cart
	clearCalls []string
	err        error
}

func (m *mockCartClient) GetCart(ctx context.Context, userID string) (*cartpb.Cart, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	c, ok := m.carts[userID]
	if !ok {
		return nil, status.Error(codes.NotFound, "cart not found")
	}
	return c, nil
}

func (m *mockCartClient) ClearCart(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.clearCalls = append(m.clearCalls, userID)
	return nil
}

// Mock Product Client
type mockProductClient struct {
	mu       sync.RWMutex
	products map[string]*productpb.Product
	err      error
}

func (m *mockProductClient) GetProduct(ctx context.Context, productID string) (*productpb.Product, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	p, ok := m.products[productID]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return p, nil
}

// Mock Inventory Client
type mockInventoryClient struct {
	mu     sync.RWMutex
	stocks map[string]*inventorypb.StockItem
	err    error
}

func (m *mockInventoryClient) GetStock(ctx context.Context, productID string) (*inventorypb.StockItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.err != nil {
		return nil, m.err
	}
	s, ok := m.stocks[productID]
	if !ok {
		return nil, status.Error(codes.NotFound, "stock not found")
	}
	return s, nil
}

func TestOrderService_CreateOrder_SingleStore_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	storeID := uuid.New().String()
	prod1ID := uuid.New().String()
	prod2ID := uuid.New().String()

	cartClient := &mockCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-1",
				UserId: userID,
				Items: []*cartpb.CartItem{
					{
						Id:        "item-1",
						ProductId: prod1ID,
						StoreId:   storeID,
						Quantity:  2,
						UnitPrice: 10.00, // untrusted cart price
					},
					{
						Id:        "item-2",
						ProductId: prod2ID,
						StoreId:   storeID,
						Quantity:  1,
						UnitPrice: 15.00, // untrusted cart price
					},
				},
			},
		},
	}

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prod1ID: {
				Id:      prod1ID,
				StoreId: storeID,
				Sku:     "SKU-SHIRT-01",
				Name:    "Cotton Shirt",
				Price:   25.00, // authoritative price
				Tax:     10.00, // 10% tax
				Status:  "PUBLISHED",
			},
			prod2ID: {
				Id:      prod2ID,
				StoreId: storeID,
				Sku:     "SKU-PANTS-01",
				Name:    "Denim Jeans",
				Price:   50.00, // authoritative price
				Tax:     5.00,  // 5% tax
				Status:  "IN_STOCK",
			},
		},
	}

	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prod1ID: {ProductId: prod1ID, AvailableQuantity: 10},
			prod2ID: {ProductId: prod2ID, AvailableQuantity: 5},
		},
	}

	svc := service.NewOrderService(repo, cartClient, prodClient, invClient, 5, nil)

	shippingAddr := "123 Main St, New York, NY 10001"
	shipAddrID := uuid.New().String()
	billAddrID := uuid.New().String()

	result, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:            userID,
		IdempotencyKey:    uuid.New().String(),
		ShippingAddress:   shippingAddr,
		ShippingAddressID: shipAddrID,
		BillingAddressID:  billAddrID,
		ShippingFee:       15.00,
		DiscountAmount:    5.00,
		Currency:          "USD",
	})

	if err != nil {
		t.Fatalf("expected CreateOrder to succeed, got %v", err)
	}

	if result == nil || result.PrimaryOrder == nil {
		t.Fatalf("expected non-nil primary order")
	}

	order := result.PrimaryOrder
	if order.Status != model.OrderStatusCreated {
		t.Errorf("expected order status CREATED, got %s", order.Status)
	}
	if order.PaymentStatus != model.PaymentStatusPending {
		t.Errorf("expected payment status PENDING, got %s", order.PaymentStatus)
	}
	if order.UserID != userID {
		t.Errorf("expected user_id %s, got %s", userID, order.UserID)
	}
	if order.StoreID != storeID {
		t.Errorf("expected store_id %s, got %s", storeID, order.StoreID)
	}
	if order.ParentOrderID == nil || *order.ParentOrderID == "" {
		t.Errorf("expected valid parent_order_id, got nil or empty")
	}
	if order.ParentOrderID != nil && *order.ParentOrderID != result.ParentOrderID {
		t.Errorf("expected parent_order_id match, got %s vs %s", *order.ParentOrderID, result.ParentOrderID)
	}

	// Calculations:
	// item 1: 2 * 25.00 = 50.00 subtotal, tax 10% = 5.00
	// item 2: 1 * 50.00 = 50.00 subtotal, tax 5% = 2.50
	// subtotal = 100.00
	// tax_amount = 7.50
	// shipping_fee = 15.00
	// discount = 5.00
	// total = 100.00 + 7.50 + 15.00 - 5.00 = 117.50
	if order.Subtotal != 100.00 {
		t.Errorf("expected subtotal 100.00, got %.2f", order.Subtotal)
	}
	if order.TaxAmount != 7.50 {
		t.Errorf("expected tax_amount 7.50, got %.2f", order.TaxAmount)
	}
	if order.ShippingFee != 15.00 {
		t.Errorf("expected shipping_fee 15.00, got %.2f", order.ShippingFee)
	}
	if order.DiscountAmount != 5.00 {
		t.Errorf("expected discount_amount 5.00, got %.2f", order.DiscountAmount)
	}
	if order.TotalAmount != 117.50 {
		t.Errorf("expected total_amount 117.50, got %.2f", order.TotalAmount)
	}

	// Verify snapshots in order items
	if len(order.Items) != 2 {
		t.Fatalf("expected 2 order items, got %d", len(order.Items))
	}
	for _, it := range order.Items {
		if it.ProductID == prod1ID {
			if it.UnitPrice != 25.00 {
				t.Errorf("expected item 1 unit price 25.00, got %.2f", it.UnitPrice)
			}
			if it.ProductName != "Cotton Shirt" {
				t.Errorf("expected snapshot name Cotton Shirt, got %s", it.ProductName)
			}
			if it.SKU != "SKU-SHIRT-01" {
				t.Errorf("expected snapshot SKU SKU-SHIRT-01, got %s", it.SKU)
			}
		}
	}

	// Verify outbox event was recorded
	if len(repo.outboxEnvelopes) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(repo.outboxEnvelopes))
	}
	if repo.outboxTopic != events.TopicOrderCreated {
		t.Errorf("expected outbox topic %s, got %s", events.TopicOrderCreated, repo.outboxTopic)
	}
	if repo.outboxEnvelopes[0].EventType != events.EventTypeOrderCreated {
		t.Errorf("expected EventType OrderCreated, got %s", repo.outboxEnvelopes[0].EventType)
	}
	if repo.outboxEnvelopes[0].AggregateID != order.ID {
		t.Errorf("expected AggregateID %s, got %s", order.ID, repo.outboxEnvelopes[0].AggregateID)
	}
}

func TestOrderService_CreateOrder_MultiStoreSplit(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	storeA := uuid.New().String()
	storeB := uuid.New().String()
	prodA1 := uuid.New().String()
	prodA2 := uuid.New().String()
	prodB1 := uuid.New().String()

	cartClient := &mockCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-multi",
				UserId: userID,
				Items: []*cartpb.CartItem{
					{ProductId: prodA1, StoreId: storeA, Quantity: 2},
					{ProductId: prodA2, StoreId: storeA, Quantity: 1},
					{ProductId: prodB1, StoreId: storeB, Quantity: 3},
				},
			},
		},
	}

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prodA1: {Id: prodA1, StoreId: storeA, Sku: "SKU-A1", Name: "Prod A1", Price: 10.00, Status: "PUBLISHED"},
			prodA2: {Id: prodA2, StoreId: storeA, Sku: "SKU-A2", Name: "Prod A2", Price: 20.00, Status: "PUBLISHED"},
			prodB1: {Id: prodB1, StoreId: storeB, Sku: "SKU-B1", Name: "Prod B1", Price: 30.00, Status: "PUBLISHED"},
		},
	}

	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodA1: {ProductId: prodA1, AvailableQuantity: 10},
			prodA2: {ProductId: prodA2, AvailableQuantity: 10},
			prodB1: {ProductId: prodB1, AvailableQuantity: 10},
		},
	}

	svc := service.NewOrderService(repo, cartClient, prodClient, invClient, 5, nil)

	result, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "456 Market St, San Francisco, CA",
		ShippingFee:     20.00, // $10 per store
	})

	if err != nil {
		t.Fatalf("expected CreateOrder to succeed for multi-store cart, got %v", err)
	}

	if len(result.Orders) != 2 {
		t.Fatalf("expected 2 split orders for 2 distinct stores, got %d", len(result.Orders))
	}

	if result.ParentOrderID == "" {
		t.Fatalf("expected common parent_order_id, got empty")
	}

	// Verify both orders share the exact same parent_order_id
	var storeAOrder, storeBOrder *model.Order
	for _, o := range result.Orders {
		if o.ParentOrderID == nil || *o.ParentOrderID != result.ParentOrderID {
			t.Errorf("order %s does not share parent_order_id %s", o.ID, result.ParentOrderID)
		}
		if o.Status != model.OrderStatusCreated {
			t.Errorf("expected status CREATED, got %s", o.Status)
		}
		switch o.StoreID {
		case storeA:
			storeAOrder = o
		case storeB:
			storeBOrder = o
		}
	}

	if storeAOrder == nil || storeBOrder == nil {
		t.Fatalf("expected orders for both storeA and storeB, got nil")
	}

	// Store A: 2 * 10 + 1 * 20 = 40.00 subtotal, shipping = 10.00, total = 50.00
	if storeAOrder.Subtotal != 40.00 {
		t.Errorf("expected Store A subtotal 40.00, got %.2f", storeAOrder.Subtotal)
	}
	if storeAOrder.ShippingFee != 10.00 {
		t.Errorf("expected Store A shipping_fee 10.00, got %.2f", storeAOrder.ShippingFee)
	}
	if len(storeAOrder.Items) != 2 {
		t.Errorf("expected Store A to have 2 items, got %d", len(storeAOrder.Items))
	}

	// Store B: 3 * 30 = 90.00 subtotal, shipping = 10.00, total = 100.00
	if storeBOrder.Subtotal != 90.00 {
		t.Errorf("expected Store B subtotal 90.00, got %.2f", storeBOrder.Subtotal)
	}
	if storeBOrder.ShippingFee != 10.00 {
		t.Errorf("expected Store B shipping_fee 10.00, got %.2f", storeBOrder.ShippingFee)
	}
	if len(storeBOrder.Items) != 1 {
		t.Errorf("expected Store B to have 1 item, got %d", len(storeBOrder.Items))
	}

	// Verify 2 outbox events persisted with matching parent_order_id
	if len(repo.outboxEnvelopes) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(repo.outboxEnvelopes))
	}
}

func TestOrderService_CreateOrder_AuthoritativePriceEnforced(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	storeID := uuid.New().String()
	prodID := uuid.New().String()

	// Client passes item with forged price of 1.00
	input := dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "789 Broadway, NYC",
		Items: []dto.CreateOrderItemInput{
			{
				ProductID: prodID,
				StoreID:   storeID,
				Quantity:  2,
				UnitPrice: 1.00, // manipulated price
			},
		},
	}

	// Authoritative price is 99.99
	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prodID: {
				Id:      prodID,
				StoreId: storeID,
				Sku:     "AUTH-SKU",
				Name:    "Expensive Item",
				Price:   99.99,
				Status:  "PUBLISHED",
			},
		},
	}

	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 10},
		},
	}

	svc := service.NewOrderService(repo, nil, prodClient, invClient, 5, nil)

	result, err := svc.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("expected CreateOrder to succeed, got %v", err)
	}

	order := result.PrimaryOrder
	if order.Subtotal != 199.98 {
		t.Errorf("expected authoritative subtotal 199.98, got %.2f (price manipulation was not blocked!)", order.Subtotal)
	}
	if order.Items[0].UnitPrice != 99.99 {
		t.Errorf("expected item unit price 99.99, got %.2f", order.Items[0].UnitPrice)
	}
}

func TestOrderService_CreateOrder_EmptyCartRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	userID := uuid.New().String()

	cartClient := &mockCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {Id: "c-empty", UserId: userID, Items: []*cartpb.CartItem{}},
		},
	}

	svc := service.NewOrderService(repo, cartClient, nil, nil, 5, nil)

	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "123 Street",
	})

	if err == nil {
		t.Fatalf("expected empty cart to be rejected, got nil error")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %v", err)
	}
}

func TestOrderService_CreateOrder_InvalidUserRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	// Missing user_id
	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          "",
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "123 Street",
	})
	if err == nil {
		t.Fatalf("expected missing user_id to fail")
	}

	// Non-UUID user_id
	_, err = svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          "not-a-valid-uuid",
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "123 Street",
	})
	if err == nil {
		t.Fatalf("expected non-UUID user_id to fail")
	}
}

func TestOrderService_CreateOrder_MissingAddressRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:         uuid.New().String(),
		IdempotencyKey: uuid.New().String(),
	})
	if err == nil {
		t.Fatalf("expected missing address to fail")
	}
}

func TestOrderService_CreateOrder_InsufficientStockRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	prodID := uuid.New().String()
	storeID := uuid.New().String()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prodID: {Id: prodID, StoreId: storeID, Price: 10.00, Status: "PUBLISHED"},
		},
	}

	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 2}, // only 2 available
		},
	}

	svc := service.NewOrderService(repo, nil, prodClient, invClient, 5, nil)

	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "123 Main St",
		Items: []dto.CreateOrderItemInput{
			{ProductID: prodID, StoreID: storeID, Quantity: 5}, // requesting 5
		},
	})

	if err == nil {
		t.Fatalf("expected insufficient stock to be rejected, got nil error")
	}
	if !appErrors.IsConflict(err) {
		t.Errorf("expected Conflict error for insufficient stock, got %v", err)
	}
}

func TestOrderService_CreateOrder_InactiveProductRejected(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	prodID := uuid.New().String()
	storeID := uuid.New().String()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prodID: {Id: prodID, StoreId: storeID, Price: 10.00, Status: "DRAFT"}, // not active
		},
	}

	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 10},
		},
	}

	svc := service.NewOrderService(repo, nil, prodClient, invClient, 5, nil)

	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "123 Main St",
		Items: []dto.CreateOrderItemInput{
			{ProductID: prodID, StoreID: storeID, Quantity: 1},
		},
	})

	if err == nil {
		t.Fatalf("expected inactive product to be rejected, got nil error")
	}
}

func TestOrderService_GetOrder_And_UpdateStatus(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	orderID := uuid.New().String()
	repo.orders[orderID] = &model.Order{
		ID:          orderID,
		OrderNumber: "ORD-TEST-001",
		UserID:      uuid.New().String(),
		StoreID:     uuid.New().String(),
		Status:      model.OrderStatusCreated,
		TotalAmount: 100.00,
		Items:       []model.OrderItem{},
	}

	// 1. GetOrder
	order, err := svc.GetOrder(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if order.ID != orderID {
		t.Errorf("expected order id %s, got %s", orderID, order.ID)
	}

	// 2. Valid UpdateOrderStatus: CREATED -> PENDING_PAYMENT
	updated, err := svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusPendingPayment)
	if err != nil {
		t.Fatalf("failed to update order status: %v", err)
	}
	if updated.Status != model.OrderStatusPendingPayment {
		t.Errorf("expected PENDING_PAYMENT status, got %s", updated.Status)
	}

	// 3. Invalid transition: PENDING_PAYMENT -> DELIVERED (skipping states is rejected)
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
	if err == nil {
		t.Fatalf("expected invalid transition to DELIVERED to be rejected")
	}

	// 4. Update with unknown status
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatus("UNKNOWN_STATUS"))
	if err == nil {
		t.Fatalf("expected error on unknown status")
	}
}

func TestOrderService_GetOrder_Authorization(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	ownerID := uuid.New().String()
	otherUserID := uuid.New().String()
	orderID := uuid.New().String()

	repo.orders[orderID] = &model.Order{
		ID:          orderID,
		UserID:      ownerID,
		StoreID:     uuid.New().String(),
		Status:      model.OrderStatusCreated,
		TotalAmount: 50.00,
	}

	// 1. Owner can get own order
	ord, err := svc.GetOrder(ctx, orderID, ownerID)
	if err != nil {
		t.Fatalf("expected owner to be able to get order, got %v", err)
	}
	if ord.ID != orderID {
		t.Errorf("expected order id %s, got %s", orderID, ord.ID)
	}

	// 2. Different user receives Forbidden error
	_, err = svc.GetOrder(ctx, orderID, otherUserID)
	if err == nil {
		t.Fatalf("expected different user to be forbidden, got nil error")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN error code, got %v", err)
	}
}

func TestOrderService_CancelOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	ownerID := uuid.New().String()
	otherUserID := uuid.New().String()
	orderID := uuid.New().String()

	repo.orders[orderID] = &model.Order{
		ID:          orderID,
		UserID:      ownerID,
		StoreID:     uuid.New().String(),
		Status:      model.OrderStatusCreated,
		TotalAmount: 50.00,
	}

	// 1. Unauthorized cancellation by different user
	_, err := svc.CancelOrder(ctx, orderID, otherUserID, "Reason")
	if err == nil {
		t.Fatalf("expected different user cancellation to fail with forbidden")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN error code, got %v", err)
	}

	// 2. Successful cancellation by owner
	cancelled, err := svc.CancelOrder(ctx, orderID, ownerID, "Changed mind")
	if err != nil {
		t.Fatalf("expected cancellation to succeed, got %v", err)
	}
	if cancelled.Status != model.OrderStatusCancelled {
		t.Errorf("expected CANCELLED status, got %s", cancelled.Status)
	}

	// 3. Cancelling already cancelled order should fail
	_, err = svc.CancelOrder(ctx, orderID, ownerID, "Cancel again")
	if err == nil {
		t.Fatalf("expected cancelling already cancelled order to fail")
	}

	// 4. Shipped order cannot be cancelled
	shippedOrderID := uuid.New().String()
	repo.orders[shippedOrderID] = &model.Order{
		ID:          shippedOrderID,
		UserID:      ownerID,
		Status:      model.OrderStatusShipped,
		TotalAmount: 50.00,
	}
	_, err = svc.CancelOrder(ctx, shippedOrderID, ownerID, "Cancel shipped")
	if err == nil {
		t.Fatalf("expected cancelling shipped order to fail")
	}
}

func TestOrderService_CreateOrder_MissingIdempotencyKey(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	_, err := svc.CreateOrder(ctx, dto.CreateOrderInput{
		UserID:          uuid.New().String(),
		ShippingAddress: "123 Main St",
		// IdempotencyKey omitted
	})
	if err == nil {
		t.Fatalf("expected missing idempotency key to be rejected")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code for missing idempotency key, got %v", err)
	}
}

func TestOrderService_CreateOrder_Idempotency_DuplicateRequest(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()

	userID := uuid.New().String()
	otherUserID := uuid.New().String()
	prodID := uuid.New().String()
	storeID := uuid.New().String()
	idempKey := "idemp-key-" + uuid.New().String()

	prodClient := &mockProductClient{
		products: map[string]*productpb.Product{
			prodID: {Id: prodID, StoreId: storeID, Price: 25.00, Status: "PUBLISHED"},
		},
	}
	invClient := &mockInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prodID: {ProductId: prodID, AvailableQuantity: 20},
		},
	}

	svc := service.NewOrderService(repo, nil, prodClient, invClient, 5, nil)

	input := dto.CreateOrderInput{
		UserID:          userID,
		IdempotencyKey:  idempKey,
		ShippingAddress: "100 Pine St, Seattle, WA",
		Items: []dto.CreateOrderItemInput{
			{ProductID: prodID, StoreID: storeID, Quantity: 2},
		},
	}

	// 1. First order creation request: succeeds
	res1, err := svc.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("expected first CreateOrder to succeed, got %v", err)
	}
	if res1 == nil || res1.PrimaryOrder == nil {
		t.Fatalf("expected non-nil primary order on first request")
	}
	firstOrderID := res1.PrimaryOrder.ID

	// Verify exactly 1 order in repository
	if len(repo.orders) != 1 {
		t.Fatalf("expected 1 order in repo, got %d", len(repo.orders))
	}

	// 2. Duplicate order creation request with same idempotency key: succeeds idempotently
	res2, err := svc.CreateOrder(ctx, input)
	if err != nil {
		t.Fatalf("expected duplicate CreateOrder to succeed idempotently, got %v", err)
	}
	if res2 == nil || res2.PrimaryOrder == nil {
		t.Fatalf("expected non-nil primary order on duplicate request")
	}
	if res2.PrimaryOrder.ID != firstOrderID {
		t.Errorf("expected duplicate request to return original order id %s, got %s", firstOrderID, res2.PrimaryOrder.ID)
	}

	// Crucial check: verify NO duplicate orders were created
	if len(repo.orders) != 1 {
		t.Errorf("expected still exactly 1 order in repository after duplicate request, got %d", len(repo.orders))
	}

	// 3. Duplicate idempotency key used by a different user: rejected with Conflict
	inputDiffUser := input
	inputDiffUser.UserID = otherUserID
	_, err = svc.CreateOrder(ctx, inputDiffUser)
	if err == nil {
		t.Fatalf("expected duplicate idempotency key by different user to fail with Conflict")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeConflict {
		t.Errorf("expected Conflict error code, got %v", err)
	}
}

func TestOrderService_OrderStateMachine_Transitions(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	orderID := uuid.New().String()
	repo.orders[orderID] = &model.Order{
		ID:          orderID,
		OrderNumber: "ORD-STATE-001",
		UserID:      uuid.New().String(),
		StoreID:     uuid.New().String(),
		Status:      model.OrderStatusCreated,
		TotalAmount: 150.00,
	}

	// 1. CREATED -> PENDING_PAYMENT
	o, err := svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusPendingPayment)
	if err != nil {
		t.Fatalf("expected transition CREATED -> PENDING_PAYMENT to succeed: %v", err)
	}
	if o.Status != model.OrderStatusPendingPayment {
		t.Errorf("expected PENDING_PAYMENT, got %s", o.Status)
	}

	// 2. Invalid backward or skipping transition: PENDING_PAYMENT -> SHIPPED (rejected!)
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusShipped)
	if err == nil {
		t.Fatalf("expected PENDING_PAYMENT -> SHIPPED to be rejected as invalid transition")
	}

	// 3. PENDING_PAYMENT -> PAID
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusPaid)
	if err != nil {
		t.Fatalf("expected transition PENDING_PAYMENT -> PAID to succeed: %v", err)
	}
	if o.Status != model.OrderStatusPaid {
		t.Errorf("expected PAID, got %s", o.Status)
	}

	// 4. PAID -> CONFIRMED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusConfirmed)
	if err != nil {
		t.Fatalf("expected transition PAID -> CONFIRMED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusConfirmed {
		t.Errorf("expected CONFIRMED, got %s", o.Status)
	}

	// 5. CONFIRMED -> PROCESSING
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusProcessing)
	if err != nil {
		t.Fatalf("expected transition CONFIRMED -> PROCESSING to succeed: %v", err)
	}
	if o.Status != model.OrderStatusProcessing {
		t.Errorf("expected PROCESSING, got %s", o.Status)
	}

	// 6. PROCESSING -> SHIPPED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusShipped)
	if err != nil {
		t.Fatalf("expected transition PROCESSING -> SHIPPED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusShipped {
		t.Errorf("expected SHIPPED, got %s", o.Status)
	}

	// 7. SHIPPED -> OUT_FOR_DELIVERY
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusOutForDelivery)
	if err != nil {
		t.Fatalf("expected transition SHIPPED -> OUT_FOR_DELIVERY to succeed: %v", err)
	}
	if o.Status != model.OrderStatusOutForDelivery {
		t.Errorf("expected OUT_FOR_DELIVERY, got %s", o.Status)
	}

	// 8. OUT_FOR_DELIVERY -> DELIVERED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
	if err != nil {
		t.Fatalf("expected transition OUT_FOR_DELIVERY -> DELIVERED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusDelivered {
		t.Errorf("expected DELIVERED, got %s", o.Status)
	}

	// 9. DELIVERED -> RETURN_REQUESTED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusReturnRequested)
	if err != nil {
		t.Fatalf("expected transition DELIVERED -> RETURN_REQUESTED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusReturnRequested {
		t.Errorf("expected RETURN_REQUESTED, got %s", o.Status)
	}

	// 10. RETURN_REQUESTED -> RETURNED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusReturned)
	if err != nil {
		t.Fatalf("expected transition RETURN_REQUESTED -> RETURNED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusReturned {
		t.Errorf("expected RETURNED, got %s", o.Status)
	}

	// 11. RETURNED -> REFUNDED
	o, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusRefunded)
	if err != nil {
		t.Fatalf("expected transition RETURNED -> REFUNDED to succeed: %v", err)
	}
	if o.Status != model.OrderStatusRefunded {
		t.Errorf("expected REFUNDED, got %s", o.Status)
	}

	// 12. Terminal state: REFUNDED cannot transition to anything
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusDelivered)
	if err == nil {
		t.Fatalf("expected transition from terminal REFUNDED to be rejected")
	}

	// 13. Test failure path: PENDING_PAYMENT -> PAYMENT_FAILED
	failedOrderID := uuid.New().String()
	repo.orders[failedOrderID] = &model.Order{
		ID:          failedOrderID,
		Status:      model.OrderStatusPendingPayment,
		TotalAmount: 80.00,
	}
	failedOrd, err := svc.UpdateOrderStatus(ctx, failedOrderID, model.OrderStatusPaymentFailed)
	if err != nil {
		t.Fatalf("expected PENDING_PAYMENT -> PAYMENT_FAILED to succeed: %v", err)
	}
	if failedOrd.Status != model.OrderStatusPaymentFailed {
		t.Errorf("expected PAYMENT_FAILED, got %s", failedOrd.Status)
	}
}

func TestOrderService_Cancellation_BeforeAndAfterShipment(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	userID := uuid.New().String()

	// 1. Cancellation before shipment must succeed for all pre-shipment states
	preShipmentStates := []model.OrderStatus{
		model.OrderStatusCreated,
		model.OrderStatusPendingPayment,
		model.OrderStatusPaid,
		model.OrderStatusConfirmed,
		model.OrderStatusProcessing,
	}

	for _, st := range preShipmentStates {
		ordID := uuid.New().String()
		repo.orders[ordID] = &model.Order{
			ID:          ordID,
			UserID:      userID,
			Status:      st,
			TotalAmount: 50.00,
		}

		cancelled, err := svc.CancelOrder(ctx, ordID, userID, "Customer request before shipment")
		if err != nil {
			t.Errorf("expected cancellation from %s to succeed, got %v", st, err)
			continue
		}
		if cancelled.Status != model.OrderStatusCancelled {
			t.Errorf("expected CANCELLED status, got %s", cancelled.Status)
		}
	}

	// 2. Cancellation at or after shipment must be REJECTED
	postShipmentStates := []model.OrderStatus{
		model.OrderStatusShipped,
		model.OrderStatusOutForDelivery,
		model.OrderStatusDelivered,
		model.OrderStatusReturnRequested,
		model.OrderStatusReturned,
		model.OrderStatusRefunded,
	}

	for _, st := range postShipmentStates {
		ordID := uuid.New().String()
		repo.orders[ordID] = &model.Order{
			ID:          ordID,
			UserID:      userID,
			Status:      st,
			TotalAmount: 50.00,
		}

		_, err := svc.CancelOrder(ctx, ordID, userID, "Customer cancel attempt")
		if err == nil {
			t.Errorf("expected cancellation from %s to be rejected, but it succeeded", st)
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
			t.Errorf("expected BAD_REQUEST for cancellation from %s, got %v", st, err)
		}
	}
}

func TestOrderService_OrderStatusHistory_Traceable(t *testing.T) {
	ctx := context.Background()
	repo := newMockOrderRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)

	userID := uuid.New().String()
	orderID := uuid.New().String()

	repo.orders[orderID] = &model.Order{
		ID:          orderID,
		UserID:      userID,
		Status:      model.OrderStatusCreated,
		TotalAmount: 75.00,
		CreatedAt:   time.Now().UTC(),
	}

	// Record initial CREATED history
	repo.history[orderID] = append(repo.history[orderID], &model.OrderStatusHistory{
		ID:        uuid.New().String(),
		OrderID:   orderID,
		ToStatus:  model.OrderStatusCreated,
		Reason:    "Order created",
		ChangedBy: userID,
		CreatedAt: time.Now().UTC(),
	})

	// Transition 1: CREATED -> PENDING_PAYMENT
	_, err := svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusPendingPayment, "Payment pending redirect")
	if err != nil {
		t.Fatalf("failed transition to PENDING_PAYMENT: %v", err)
	}

	// Transition 2: PENDING_PAYMENT -> PAID
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusPaid, "Gateway webhook payment success")
	if err != nil {
		t.Fatalf("failed transition to PAID: %v", err)
	}

	// Transition 3: PAID -> CONFIRMED
	_, err = svc.UpdateOrderStatus(ctx, orderID, model.OrderStatusConfirmed, "Merchant accepted order")
	if err != nil {
		t.Fatalf("failed transition to CONFIRMED: %v", err)
	}

	// Fetch status history
	history, err := svc.GetOrderStatusHistory(ctx, orderID)
	if err != nil {
		t.Fatalf("failed to retrieve status history: %v", err)
	}

	// Verify all transitions are traceable
	if len(history) != 4 {
		t.Fatalf("expected 4 status history entries, got %d", len(history))
	}

	expectedTransitions := []struct {
		from *model.OrderStatus
		to   model.OrderStatus
	}{
		{from: nil, to: model.OrderStatusCreated},
		{from: ptr(model.OrderStatusCreated), to: model.OrderStatusPendingPayment},
		{from: ptr(model.OrderStatusPendingPayment), to: model.OrderStatusPaid},
		{from: ptr(model.OrderStatusPaid), to: model.OrderStatusConfirmed},
	}

	for i, exp := range expectedTransitions {
		entry := history[i]
		if entry.ToStatus != exp.to {
			t.Errorf("entry %d: expected to_status %s, got %s", i, exp.to, entry.ToStatus)
		}
		if exp.from == nil {
			if entry.FromStatus != nil {
				t.Errorf("entry %d: expected nil from_status, got %s", i, *entry.FromStatus)
			}
		} else {
			if entry.FromStatus == nil || *entry.FromStatus != *exp.from {
				t.Errorf("entry %d: expected from_status %v, got %v", i, exp.from, entry.FromStatus)
			}
		}
	}
}

func ptr[T any](v T) *T {
	return &v
}


