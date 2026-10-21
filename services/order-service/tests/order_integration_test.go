package tests

import (
	"context"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type inMemOrderRepo struct {
	mu      sync.RWMutex
	orders  map[string]*model.Order
	history map[string][]*model.OrderStatusHistory
	outbox  []*events.EventEnvelope
	topic   string
}

func newInMemOrderRepo() *inMemOrderRepo {
	return &inMemOrderRepo{
		orders:  make(map[string]*model.Order),
		history: make(map[string][]*model.OrderStatusHistory),
		outbox:  make([]*events.EventEnvelope, 0),
	}
}

func (m *inMemOrderRepo) CreateOrdersWithOutbox(
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
	m.outbox = append(m.outbox, envelopes...)
	m.topic = topic
	return nil
}

func (m *inMemOrderRepo) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
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

func (m *inMemOrderRepo) GetOrdersByIdempotencyKey(ctx context.Context, idempotencyKey string) ([]*model.Order, error) {
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

func (m *inMemOrderRepo) ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Order, int, error) {
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
	return res, len(res), nil
}

func (m *inMemOrderRepo) UpdateOrderStatus(ctx context.Context, id string, orderStatus model.OrderStatus, reason ...string) (*model.Order, error) {
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
	return &cp, nil
}

func (m *inMemOrderRepo) CancelOrder(ctx context.Context, id string, envelope *events.EventEnvelope, topic string, reason ...string) (*model.Order, error) {
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
		m.outbox = append(m.outbox, envelope)
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
	return &cp, nil
}

func (m *inMemOrderRepo) GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error) {
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

func (m *inMemOrderRepo) CreateOrderWithOutbox(
	ctx context.Context,
	orderID string,
	insertOrder func(ctx context.Context, tx pgx.Tx) error,
	envelope *events.EventEnvelope,
	topic string,
) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.outbox = append(m.outbox, envelope)
	return nil
}

func (m *inMemOrderRepo) getOutboxCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.outbox)
}

type inMemCartClient struct {
	mu    sync.RWMutex
	carts map[string]*cartpb.Cart
}

func (c *inMemCartClient) GetCart(ctx context.Context, userID string) (*cartpb.Cart, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	cart, ok := c.carts[userID]
	if !ok {
		return nil, status.Error(codes.NotFound, "cart not found")
	}
	return cart, nil
}

func (c *inMemCartClient) ClearCart(ctx context.Context, userID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.carts, userID)
	return nil
}

func (c *inMemCartClient) setCart(userID string, cart *cartpb.Cart) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.carts[userID] = cart
}

type inMemProductClient struct {
	mu       sync.RWMutex
	products map[string]*productpb.Product
}

func (p *inMemProductClient) GetProduct(ctx context.Context, id string) (*productpb.Product, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	prod, ok := p.products[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return prod, nil
}

type inMemInventoryClient struct {
	mu     sync.RWMutex
	stocks map[string]*inventorypb.StockItem
}

func (i *inMemInventoryClient) GetStock(ctx context.Context, id string) (*inventorypb.StockItem, error) {
	i.mu.RLock()
	defer i.mu.RUnlock()
	st, ok := i.stocks[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "stock not found")
	}
	return st, nil
}

func (i *inMemInventoryClient) setStockQuantity(id string, qty int32) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if st, ok := i.stocks[id]; ok {
		st.AvailableQuantity = qty
	}
}

func setupInProcessGRPC(
	t *testing.T,
	repo repository.OrderRepository,
	cartClient *inMemCartClient,
	prodClient *inMemProductClient,
	invClient *inMemInventoryClient,
) (orderpb.OrderServiceClient, func()) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	svc := service.NewOrderService(repo, cartClient, prodClient, invClient, 5, nil)
	grpcHandler := handler.NewOrderGRPCHandler(svc, nil)

	s := grpc.NewServer()
	orderpb.RegisterOrderServiceServer(s, grpcHandler)

	go func() {
		if err := s.Serve(lis); err != nil && err.Error() != "closed" {
			t.Errorf("Server error: %v", err)
		}
	}()

	conn, err := grpc.NewClient(
		"passthrough://bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return lis.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("failed to dial bufnet: %v", err)
	}

	cleanup := func() {
		_ = conn.Close()
		s.Stop()
		_ = lis.Close()
	}

	return orderpb.NewOrderServiceClient(conn), cleanup
}

func TestOrderIntegration_EndToEnd_CheckoutValidationAndSplitting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := newInMemOrderRepo()

	userID := uuid.New().String()
	store1 := uuid.New().String()
	store2 := uuid.New().String()
	prod1 := uuid.New().String()
	prod2 := uuid.New().String()

	cartClient := &inMemCartClient{
		carts: map[string]*cartpb.Cart{
			userID: {
				Id:     "cart-test",
				UserId: userID,
				Items: []*cartpb.CartItem{
					{ProductId: prod1, StoreId: store1, Quantity: 2, UnitPrice: 999.0}, // manipulated client price
					{ProductId: prod2, StoreId: store2, Quantity: 1, UnitPrice: 999.0}, // manipulated client price
				},
			},
		},
	}

	prodClient := &inMemProductClient{
		products: map[string]*productpb.Product{
			prod1: {
				Id:      prod1,
				StoreId: store1,
				Sku:     "SKU-P1",
				Name:    "Store 1 Product",
				Price:   30.00, // authoritative price
				Tax:     10.00, // 10%
				Status:  "PUBLISHED",
			},
			prod2: {
				Id:      prod2,
				StoreId: store2,
				Sku:     "SKU-P2",
				Name:    "Store 2 Product",
				Price:   60.00, // authoritative price
				Tax:     5.00,  // 5%
				Status:  "PUBLISHED",
			},
		},
	}

	invClient := &inMemInventoryClient{
		stocks: map[string]*inventorypb.StockItem{
			prod1: {ProductId: prod1, AvailableQuantity: 15},
			prod2: {ProductId: prod2, AvailableQuantity: 8},
		},
	}

	client, cleanup := setupInProcessGRPC(t, repo, cartClient, prodClient, invClient)
	defer cleanup()

	// 1. Successful Multi-Store Checkout
	resp, err := client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		CartId:          "cart-test",
		ShippingAddress: "100 Innovation Way, Suite 400",
		ShippingFee:     10.00,
	})
	if err != nil {
		t.Fatalf("expected CreateOrder to succeed, got %v", err)
	}

	if resp.GetParentOrderId() == "" {
		t.Fatalf("expected parent_order_id to be populated")
	}

	if len(resp.GetOrders()) != 2 {
		t.Fatalf("expected 2 orders for multi-store checkout, got %d", len(resp.GetOrders()))
	}

	// Verify both orders share parent_order_id
	for _, o := range resp.GetOrders() {
		if o.GetParentOrderId() != resp.GetParentOrderId() {
			t.Errorf("order %s has parent_order_id %s, expected %s", o.GetId(), o.GetParentOrderId(), resp.GetParentOrderId())
		}
		if o.GetOrderStatus() != "CREATED" {
			t.Errorf("expected initial status CREATED, got %s", o.GetOrderStatus())
		}
		if o.GetPaymentStatus() != "PENDING" {
			t.Errorf("expected payment status PENDING, got %s", o.GetPaymentStatus())
		}
	}

	// Verify store 1 order: 2 * 30.00 = 60.00 subtotal, 10% tax = 6.00, shipping = 5.00 -> total = 71.00
	var order1, order2 *orderpb.Order
	for _, o := range resp.GetOrders() {
		if o.GetStoreId() == store1 {
			order1 = o
		} else if o.GetStoreId() == store2 {
			order2 = o
		}
	}

	if order1 == nil || order2 == nil {
		t.Fatalf("expected orders for both stores")
	}

	if order1.GetSubtotal() != 60.00 {
		t.Errorf("expected store 1 subtotal 60.00, got %.2f", order1.GetSubtotal())
	}
	if order1.GetTaxAmount() != 6.00 {
		t.Errorf("expected store 1 tax 6.00, got %.2f", order1.GetTaxAmount())
	}
	if order1.GetTotalAmount() != 71.00 {
		t.Errorf("expected store 1 total 71.00, got %.2f", order1.GetTotalAmount())
	}

	// Verify store 2 order: 1 * 60.00 = 60.00 subtotal, 5% tax = 3.00, shipping = 5.00 -> total = 68.00
	if order2.GetSubtotal() != 60.00 {
		t.Errorf("expected store 2 subtotal 60.00, got %.2f", order2.GetSubtotal())
	}
	if order2.GetTaxAmount() != 3.00 {
		t.Errorf("expected store 2 tax 3.00, got %.2f", order2.GetTaxAmount())
	}
	if order2.GetTotalAmount() != 68.00 {
		t.Errorf("expected store 2 total 68.00, got %.2f", order2.GetTotalAmount())
	}

	// Verify outbox events were recorded
	if repo.getOutboxCount() != 2 {
		t.Errorf("expected 2 outbox events, got %d", repo.getOutboxCount())
	}

	// 2. Checkout rejection when stock is insufficient
	invClient.setStockQuantity(prod1, 1) // only 1 available, cart needs 2
	cartClient.setCart(userID, &cartpb.Cart{
		Id:     "cart-test",
		UserId: userID,
		Items: []*cartpb.CartItem{
			{ProductId: prod1, StoreId: store1, Quantity: 2},
		},
	})

	_, err = client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "100 Innovation Way",
	})
	if status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected conflict / insufficient stock rejection, got %v", err)
	}

	// 3. Checkout rejection on empty cart
	cartClient.setCart(userID, &cartpb.Cart{
		Id:     "cart-test",
		UserId: userID,
		Items:  []*cartpb.CartItem{},
	})
	_, err = client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          userID,
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "100 Innovation Way",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument on empty cart, got %v", err)
	}

	// 4. Checkout rejection on invalid user_id format
	_, err = client.CreateOrder(ctx, &orderpb.CreateOrderRequest{
		UserId:          "invalid-user-format",
		IdempotencyKey:  uuid.New().String(),
		ShippingAddress: "100 Innovation Way",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument on non-UUID user_id, got %v", err)
	}

	// 5. Query created order
	getResp, err := client.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id: order1.GetId(),
	})
	if err != nil {
		t.Fatalf("failed to get order: %v", err)
	}
	if getResp.GetOrder().GetId() != order1.GetId() {
		t.Errorf("expected order id %s, got %s", order1.GetId(), getResp.GetOrder().GetId())
	}

	// 6. List orders
	listResp, err := client.ListOrders(ctx, &orderpb.ListOrdersRequest{
		UserId: userID,
	})
	if err != nil {
		t.Fatalf("failed to list orders: %v", err)
	}
	if listResp.GetTotal() < 2 {
		t.Errorf("expected at least 2 orders in list, got %d", listResp.GetTotal())
	}

	// 7. GetOrder authorization: another user cannot retrieve order
	otherUserID := uuid.New().String()
	_, err = client.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id:     order1.GetId(),
		UserId: otherUserID,
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied when retrieving another user's order, got %v", status.Code(err))
	}

	// 8. CancelOrder authorization: another user cannot cancel order
	_, err = client.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     order1.GetId(),
		UserId: otherUserID,
		Reason: "Illegal cancel attempt",
	})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied on unauthorized cancellation, got %v", status.Code(err))
	}

	// 9. CancelOrder success by owner
	cancelResp, err := client.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     order1.GetId(),
		UserId: userID,
		Reason: "Changed mind before shipping",
	})
	if err != nil {
		t.Fatalf("expected cancel order to succeed, got %v", err)
	}
	if cancelResp.GetOrder().GetStatus() != "CANCELLED" {
		t.Errorf("expected CANCELLED status, got %s", cancelResp.GetOrder().GetStatus())
	}

	// 10. Cannot cancel an already cancelled order
	_, err = client.CancelOrder(ctx, &orderpb.CancelOrderRequest{
		Id:     order1.GetId(),
		UserId: userID,
		Reason: "Cancelling second time",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument when cancelling already cancelled order, got %v", status.Code(err))
	}
}
