package tests

import (
	"context"
	"encoding/json"
	"net"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type inMemReturnRepo struct {
	mu           sync.RWMutex
	returns      map[string]*model.Return
	byOrderItem  map[string]*model.Return
	exchanges    map[string]*model.Exchange
	outboxEvents []*outbox.Event
}

func newInMemReturnRepo() *inMemReturnRepo {
	return &inMemReturnRepo{
		returns:      make(map[string]*model.Return),
		byOrderItem:  make(map[string]*model.Return),
		exchanges:    make(map[string]*model.Exchange),
		outboxEvents: make([]*outbox.Event, 0),
	}
}

func (m *inMemReturnRepo) CreateReturnWithOutbox(ctx context.Context, ret *model.Return, outboxEvent *outbox.Event) error {
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

func (m *inMemReturnRepo) GetReturnByID(ctx context.Context, id string) (*model.Return, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.returns[id]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	cp := *ret
	return &cp, nil
}

func (m *inMemReturnRepo) GetReturnByOrderItemID(ctx context.Context, orderItemID string) (*model.Return, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.byOrderItem[orderItemID]
	if !ok {
		return nil, repository.ErrReturnNotFound
	}
	cp := *ret
	return &cp, nil
}

func (m *inMemReturnRepo) ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error) {
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

func (m *inMemReturnRepo) UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error) {
	return m.UpdateReturnStatusWithOutbox(ctx, id, status, "")
}

func (m *inMemReturnRepo) UpdateReturnStatusWithOutbox(ctx context.Context, id string, status model.ReturnStatus, rejectionReason string, outboxEvents ...*outbox.Event) (*model.Return, error) {
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

func (m *inMemReturnRepo) CreateVerificationWithOutbox(ctx context.Context, v *model.ProductVerification, newStatus model.ReturnStatus, outboxEvent *outbox.Event) error {
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

func (m *inMemReturnRepo) GetVerificationByReturnID(ctx context.Context, returnID string) (*model.ProductVerification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	ret, ok := m.returns[returnID]
	if !ok || ret.Verification == nil {
		return nil, nil
	}
	cp := *ret.Verification
	return &cp, nil
}

func (m *inMemReturnRepo) CreateExchangeWithOutbox(ctx context.Context, exc *model.Exchange, answers []*model.ExchangeQuestionAnswer, outboxEvent *outbox.Event) error {
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

func (m *inMemReturnRepo) GetExchangeByID(ctx context.Context, id string) (*model.Exchange, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[id]
	if !ok {
		return nil, repository.ErrExchangeNotFound
	}
	cp := *exc
	return &cp, nil
}

func (m *inMemReturnRepo) GetExchangeByOrderID(ctx context.Context, orderID string) (*model.Exchange, error) {
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

func (m *inMemReturnRepo) CreateExchangeVerificationWithOutbox(ctx context.Context, v *model.ExchangeVerification, newStatus model.ExchangeStatus, finalDiscount float64, outboxEvents ...*outbox.Event) error {
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

func (m *inMemReturnRepo) GetExchangeVerification(ctx context.Context, exchangeID string) (*model.ExchangeVerification, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[exchangeID]
	if !ok || exc.Verification == nil {
		return nil, nil
	}
	cp := *exc.Verification
	return &cp, nil
}

func (m *inMemReturnRepo) GetExchangeQuestionAnswers(ctx context.Context, exchangeID string) ([]*model.ExchangeQuestionAnswer, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	exc, ok := m.exchanges[exchangeID]
	if !ok {
		return nil, nil
	}
	return exc.Answers, nil
}

func (m *inMemReturnRepo) ListReturns(ctx context.Context, limit, offset int, status, userID, returnType string) ([]*model.Return, int, error) {
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


type inMemOrderClient struct {
	mu     sync.RWMutex
	orders map[string]*orderpb.Order
}

func newInMemOrderClient() *inMemOrderClient {
	return &inMemOrderClient{orders: make(map[string]*orderpb.Order)}
}

func (c *inMemOrderClient) GetOrder(ctx context.Context, orderID, userID string) (*orderpb.Order, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	order, ok := c.orders[orderID]
	if !ok {
		return nil, status.Error(codes.NotFound, "order not found")
	}
	return order, nil
}

type inMemProductClient struct {
	mu       sync.RWMutex
	products map[string]*productpb.Product
}

func newInMemProductClient() *inMemProductClient {
	return &inMemProductClient{products: make(map[string]*productpb.Product)}
}

func (p *inMemProductClient) GetProduct(ctx context.Context, productID string) (*productpb.Product, error) {
	p.mu.RLock()
	defer p.mu.RUnlock()
	prod, ok := p.products[productID]
	if !ok {
		return nil, status.Error(codes.NotFound, "product not found")
	}
	return prod, nil
}

func setupInProcessGRPC(
	t *testing.T,
	repo repository.ReturnRepository,
	orderClient *inMemOrderClient,
	prodClient *inMemProductClient,
	returnWindowDays int,
) (returnpb.ReturnServiceClient, func()) {
	const bufSize = 1024 * 1024
	lis := bufconn.Listen(bufSize)

	svc := service.NewReturnService(repo, orderClient, prodClient, returnWindowDays, nil)
	grpcHandler := handler.NewReturnGRPCHandler(svc, nil)

	s := grpc.NewServer()
	returnpb.RegisterReturnServiceServer(s, grpcHandler)

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

	return returnpb.NewReturnServiceClient(conn), cleanup
}

func TestReturnIntegration_EndToEnd_FullFlow(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := newInMemReturnRepo()
	orderClient := newInMemOrderClient()
	prodClient := newInMemProductClient()

	client, cleanup := setupInProcessGRPC(t, repo, orderClient, prodClient, 14)
	defer cleanup()

	orderID := uuid.New().String()
	userID := uuid.New().String()
	itemID := uuid.New().String()
	productID := uuid.New().String()

	// Seed delivered order (delivered 3 days ago)
	deliveredAt := time.Now().UTC().Add(-3 * 24 * time.Hour)
	orderClient.orders[orderID] = &orderpb.Order{
		Id:        orderID,
		UserId:    userID,
		Status:    "DELIVERED",
		UpdatedAt: deliveredAt.Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{
				Id:         itemID,
				ProductId:  productID,
				Quantity:   2,
				UnitPrice:  45.0,
				TotalPrice: 90.0,
			},
		},
	}

	// Seed returnable product
	prodClient.products[productID] = &productpb.Product{
		Id:           productID,
		Name:         "Wireless Headphones",
		IsReturnable: true,
	}

	// 1. Successful return request
	resp, err := client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderID,
		OrderItemId: itemID,
		UserId:      userID,
		Reason:      "Left earcup stopped working",
	})
	if err != nil {
		t.Fatalf("RequestReturn failed: %v", err)
	}

	retOrder := resp.GetReturnOrder()
	if retOrder == nil {
		t.Fatalf("expected non-nil ReturnOrder in response")
	}
	if retOrder.GetId() == "" {
		t.Errorf("expected return order ID to be set")
	}
	if retOrder.GetStatus() != "RETURN_REQUESTED" {
		t.Errorf("expected status RETURN_REQUESTED, got %s", retOrder.GetStatus())
	}
	if retOrder.GetRefundAmount() != 90.0 {
		t.Errorf("expected refund amount 90.0, got %f", retOrder.GetRefundAmount())
	}
	if retOrder.GetOrderItemId() != itemID {
		t.Errorf("expected order item id %s, got %s", itemID, retOrder.GetOrderItemId())
	}

	// Verify persistence in repo
	saved, err := repo.GetReturnByID(ctx, retOrder.GetId())
	if err != nil {
		t.Fatalf("failed to find return in repo: %v", err)
	}
	if saved.Reason != "Left earcup stopped working" {
		t.Errorf("expected saved reason, got %s", saved.Reason)
	}

	// Verify outbox event with ReturnRequested event
	if len(repo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event, got %d", len(repo.outboxEvents))
	}
	outboxEvt := repo.outboxEvents[0]
	if outboxEvt.EventType != events.EventTypeReturnRequested {
		t.Errorf("expected event type %s, got %s", events.EventTypeReturnRequested, outboxEvt.EventType)
	}
	if outboxEvt.Topic != events.TopicReturnRequested {
		t.Errorf("expected topic %s, got %s", events.TopicReturnRequested, outboxEvt.Topic)
	}

	var env events.EventEnvelope
	if err := json.Unmarshal(outboxEvt.Payload, &env); err != nil {
		t.Fatalf("failed to unmarshal outbox envelope: %v", err)
	}
	var domainEvt events.ReturnRequestedEvent
	if err := env.UnmarshalData(&domainEvt); err != nil {
		t.Fatalf("failed to unmarshal domain event: %v", err)
	}
	if domainEvt.ReturnID != retOrder.GetId() || domainEvt.ProductID != productID || domainEvt.RefundAmount != 90.0 {
		t.Errorf("mismatched payload fields in ReturnRequested event: %+v", domainEvt)
	}

	// 2. Reject duplicate return request for the same order item
	_, err = client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderID,
		OrderItemId: itemID,
		UserId:      userID,
		Reason:      "Second attempt",
	})
	if err == nil {
		t.Fatalf("expected duplicate return to be rejected, got success")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists error code, got %v (%s)", st.Code(), st.Message())
	}

	// 3. GetReturn
	getResp, err := client.GetReturn(ctx, &returnpb.GetReturnRequest{Id: retOrder.GetId()})
	if err != nil {
		t.Fatalf("GetReturn failed: %v", err)
	}
	if getResp.GetReturnOrder().GetId() != retOrder.GetId() {
		t.Errorf("expected return ID %s, got %s", retOrder.GetId(), getResp.GetReturnOrder().GetId())
	}

	// 4. UpdateReturnStatus
	updateResp, err := client.UpdateReturnStatus(ctx, &returnpb.UpdateReturnStatusRequest{
		Id:     retOrder.GetId(),
		Status: "APPROVED",
	})
	if err != nil {
		t.Fatalf("UpdateReturnStatus failed: %v", err)
	}
	if updateResp.GetReturnOrder().GetStatus() != "APPROVED" {
		t.Errorf("expected status APPROVED, got %s", updateResp.GetReturnOrder().GetStatus())
	}
}

func TestReturnIntegration_ValidationRejections(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	repo := newInMemReturnRepo()
	orderClient := newInMemOrderClient()
	prodClient := newInMemProductClient()

	client, cleanup := setupInProcessGRPC(t, repo, orderClient, prodClient, 14)
	defer cleanup()

	orderID := "order-delivered-recently"
	userID := "user-valid"
	itemID := "item-returnable"
	productID := "prod-returnable"

	orderClient.orders[orderID] = &orderpb.Order{
		Id:        orderID,
		UserId:    userID,
		Status:    "DELIVERED",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{Id: itemID, ProductId: productID, Quantity: 1, TotalPrice: 50.0},
		},
	}
	prodClient.products[productID] = &productpb.Product{
		Id:           productID,
		IsReturnable: true,
	}

	// 1. Mandatory reason rejection
	_, err := client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderID,
		OrderItemId: itemID,
		UserId:      userID,
		Reason:      "   ",
	})
	if st, _ := status.FromError(err); st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for empty reason, got: %v", st.Code())
	}

	// 2. Non-delivered order rejection
	orderUndeliveredID := "order-undelivered"
	orderClient.orders[orderUndeliveredID] = &orderpb.Order{
		Id:        orderUndeliveredID,
		UserId:    userID,
		Status:    "SHIPPED",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{Id: "item-undelivered", ProductId: productID, Quantity: 1, TotalPrice: 50.0},
		},
	}
	_, err = client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderUndeliveredID,
		OrderItemId: "item-undelivered",
		UserId:      userID,
		Reason:      "Package hasn't arrived but want to return",
	})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition for non-delivered order, got: %v", st.Code())
	}

	// 3. Expired return window rejection (> 14 days)
	orderExpiredID := "order-expired"
	orderClient.orders[orderExpiredID] = &orderpb.Order{
		Id:        orderExpiredID,
		UserId:    userID,
		Status:    "DELIVERED",
		UpdatedAt: time.Now().UTC().Add(-20 * 24 * time.Hour).Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{Id: "item-expired", ProductId: productID, Quantity: 1, TotalPrice: 50.0},
		},
	}
	_, err = client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderExpiredID,
		OrderItemId: "item-expired",
		UserId:      userID,
		Reason:      "Returning after 20 days",
	})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition for expired return window, got: %v", st.Code())
	}

	// 4. Non-returnable product rejection
	nonReturnableProdID := "prod-non-returnable"
	prodClient.products[nonReturnableProdID] = &productpb.Product{
		Id:           nonReturnableProdID,
		IsReturnable: false,
	}
	orderNonReturnableID := "order-non-returnable"
	orderClient.orders[orderNonReturnableID] = &orderpb.Order{
		Id:        orderNonReturnableID,
		UserId:    userID,
		Status:    "DELIVERED",
		UpdatedAt: time.Now().UTC().Format(time.RFC3339),
		Items: []*orderpb.OrderItem{
			{Id: "item-perishable", ProductId: nonReturnableProdID, Quantity: 1, TotalPrice: 30.0},
		},
	}
	_, err = client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderNonReturnableID,
		OrderItemId: "item-perishable",
		UserId:      userID,
		Reason:      "Perishable item return",
	})
	if st, _ := status.FromError(err); st.Code() != codes.FailedPrecondition {
		t.Errorf("expected FailedPrecondition for non-returnable product, got: %v", st.Code())
	}

	// 5. User association mismatch rejection
	_, err = client.RequestReturn(ctx, &returnpb.RequestReturnRequest{
		OrderId:     orderID,
		OrderItemId: itemID,
		UserId:      "different-user",
		Reason:      "Hacking Alice's order",
	})
	if st, _ := status.FromError(err); st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for user mismatch, got: %v", st.Code())
	}
}
