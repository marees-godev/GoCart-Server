package consumer_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type outboxRecord struct {
	AggregateID string
	EventType   string
	Topic       string
}

type mockPaymentRepo struct {
	mu           sync.Mutex
	payments     map[string]*model.Payment
	idempMap     map[string]*model.Payment
	orderMap     map[string]*model.Payment
	refunds      map[string]*model.Refund
	processedEvs map[string]bool
	outboxEvents []outboxRecord
	nextID       int
}

func newMockPaymentRepo() *mockPaymentRepo {
	return &mockPaymentRepo{
		payments:     make(map[string]*model.Payment),
		idempMap:     make(map[string]*model.Payment),
		orderMap:     make(map[string]*model.Payment),
		refunds:      make(map[string]*model.Refund),
		processedEvs: make(map[string]bool),
	}
}

func (m *mockPaymentRepo) CreatePayment(ctx context.Context, p *model.Payment) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if _, ok := m.idempMap[p.IdempotencyKey]; ok {
		return appErrors.Conflict("duplicate idempotency key")
	}

	m.nextID++
	p.ID = fmt.Sprintf("pay_uuid_%d", m.nextID)
	if p.PaymentMethodID == nil || *p.PaymentMethodID == "" {
		pmID := fmt.Sprintf("a0eebc99-9c0b-4ef8-bb6d-6bb9bd38%04d", m.nextID)
		p.PaymentMethodID = &pmID
	}
	p.CreatedAt = time.Now()
	p.UpdatedAt = time.Now()

	pCopy := *p
	m.payments[p.ID] = &pCopy
	m.idempMap[p.IdempotencyKey] = &pCopy
	m.orderMap[p.OrderID] = &pCopy
	return nil
}

func (m *mockPaymentRepo) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.payments[id]
	if !ok {
		return nil, appErrors.NotFound("payment not found")
	}
	pCopy := *p
	return &pCopy, nil
}

func (m *mockPaymentRepo) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.orderMap[orderID]
	if !ok {
		return nil, appErrors.NotFound("payment for order not found")
	}
	pCopy := *p
	return &pCopy, nil
}

func (m *mockPaymentRepo) GetPaymentByGatewayTransactionID(ctx context.Context, gatewayTxID string) (*model.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, p := range m.payments {
		if p.GatewayTransactionID == gatewayTxID {
			pCopy := *p
			return &pCopy, nil
		}
	}
	return nil, appErrors.NotFound("payment for gateway transaction not found")
}

func (m *mockPaymentRepo) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.idempMap[key]
	if !ok {
		return nil, appErrors.NotFound("payment not found")
	}
	pCopy := *p
	return &pCopy, nil
}

func (m *mockPaymentRepo) UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string, paymentMethod ...string) (*model.Payment, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	p, ok := m.payments[id]
	if !ok {
		return nil, appErrors.NotFound("payment not found")
	}

	oldStatus := p.Status
	if oldStatus != status && !model.CanTransitionPayment(oldStatus, status) {
		return nil, appErrors.BadRequest("cannot transition payment")
	}

	p.Status = status
	if gatewayTxID != "" {
		p.GatewayTransactionID = gatewayTxID
	}
	if failureReason != "" {
		p.FailureReason = failureReason
	}
	if len(paymentMethod) > 0 && paymentMethod[0] != "" {
		p.PaymentMethod = model.NormalizePaymentMethod(paymentMethod[0])
	}
	p.UpdatedAt = time.Now()

	if oldStatus != status {
		switch status {
		case model.PaymentStatusSuccess:
			m.outboxEvents = append(m.outboxEvents, outboxRecord{
				AggregateID: p.ID,
				EventType:   events.EventTypePaymentSuccessful,
				Topic:       events.TopicPaymentSuccessful,
			})
		case model.PaymentStatusFailed:
			m.outboxEvents = append(m.outboxEvents, outboxRecord{
				AggregateID: p.ID,
				EventType:   events.EventTypePaymentFailed,
				Topic:       events.TopicPaymentFailed,
			})
		}
	}

	m.idempMap[p.IdempotencyKey] = p
	m.orderMap[p.OrderID] = p

	pCopy := *p
	return &pCopy, nil
}

func (m *mockPaymentRepo) CreateRefund(ctx context.Context, refund *model.Refund) error {
	return nil
}

func (m *mockPaymentRepo) GetRefundByID(ctx context.Context, id string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}

func (m *mockPaymentRepo) GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}

func (m *mockPaymentRepo) GetRefundByIdempotencyKey(ctx context.Context, key string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}

func (m *mockPaymentRepo) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
	return nil, nil
}

func (m *mockPaymentRepo) GetTotalRefundedAmount(ctx context.Context, paymentID string) (float64, error) {
	return 0, nil
}

func (m *mockPaymentRepo) UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error) {
	return nil, nil
}

func (m *mockPaymentRepo) SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error {
	return nil
}

func (m *mockPaymentRepo) IsWebhookProcessed(ctx context.Context, eventID string) (bool, error) {
	return false, nil
}

func (m *mockPaymentRepo) MarkWebhookProcessed(ctx context.Context, eventID string) error {
	return nil
}

type mockOrderClient struct {
	mu     sync.Mutex
	orders map[string]*client.AuthoritativeOrder
}

func newMockOrderClient() *mockOrderClient {
	return &mockOrderClient{orders: make(map[string]*client.AuthoritativeOrder)}
}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID string) (*client.AuthoritativeOrder, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ord, ok := m.orders[orderID]; ok {
		return ord, nil
	}
	return &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr-" + orderID,
		TotalAmount: 250.00,
		Currency:    "USD",
		Status:      "CREATED",
	}, nil
}

func setupConsumerTestWithGateway() (*consumer.PaymentEventConsumer, *mockPaymentRepo, *gateway.MockGateway, *mockOrderClient) {
	repo := newMockPaymentRepo()
	gw := gateway.NewMockGateway(10 * time.Millisecond)
	orderClient := newMockOrderClient()
	svc := service.NewPaymentService(repo, gw, orderClient, 2*time.Second)
	c := consumer.NewPaymentEventConsumer(svc)
	return c, repo, gw, orderClient
}

func setupConsumerTest() (*consumer.PaymentEventConsumer, *mockPaymentRepo) {
	c, repo, _, _ := setupConsumerTestWithGateway()
	return c, repo
}

func TestPaymentEventConsumer_NilEnvelope(t *testing.T) {
	c, _ := setupConsumerTest()
	err := c.HandleEvent(context.Background(), nil)
	if err == nil {
		t.Fatal("expected error for nil envelope, got nil")
	}
}

func TestPaymentEventConsumer_UnhandledEvent(t *testing.T) {
	c, _ := setupConsumerTest()
	env := &events.EventEnvelope{
		EventType: "unhandled.event.type",
	}
	err := c.HandleEvent(context.Background(), env)
	if err != nil {
		t.Fatalf("expected nil for unhandled event, got: %v", err)
	}
}

func TestPaymentEventConsumer_MalformedPayload(t *testing.T) {
	c, _ := setupConsumerTest()
	env := &events.EventEnvelope{
		EventType: events.EventTypeOrderCreated,
		Data:      []byte("invalid-json"),
	}
	err := c.HandleEvent(context.Background(), env)
	if err == nil {
		t.Fatal("expected error for malformed payload, got nil")
	}
}

func TestPaymentEventConsumer_OrderCreated_Success(t *testing.T) {
	c, repo := setupConsumerTest()

	orderID := "ord-succ-101"
	payload := events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      "usr-succ-101",
		TotalAmount: 250.00,
		Currency:    "USD",
		CreatedAt:   time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", orderID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "corr-succ-101"

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify payment record
	payment, err := repo.GetPaymentByOrderID(context.Background(), orderID)
	if err != nil {
		t.Fatalf("failed to get payment for order: %v", err)
	}
	if payment.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status SUCCESS, got: %s", payment.Status)
	}
	if payment.Amount != 250.00 {
		t.Errorf("expected amount 250.00, got: %f", payment.Amount)
	}

	// Verify outbox has PaymentSuccessful
	repo.mu.Lock()
	defer repo.mu.Unlock()
	foundSuccess := false
	for _, rec := range repo.outboxEvents {
		if rec.EventType == events.EventTypePaymentSuccessful && rec.Topic == events.TopicPaymentSuccessful {
			foundSuccess = true
			break
		}
	}
	if !foundSuccess {
		t.Errorf("expected outbox event PaymentSuccessful, got: %+v", repo.outboxEvents)
	}
}

func TestPaymentEventConsumer_OrderCreated_Failure(t *testing.T) {
	c, repo, gw, orderClient := setupConsumerTestWithGateway()

	orderID := "ord-fail-202"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr-fail-202",
		TotalAmount: 99.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	// Explicitly simulate gateway decline via mock gateway configuration
	gw.SetFailPayment(true)

	payload := events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      "usr-fail-202",
		TotalAmount: 99.00,
		Currency:    "USD",
		CreatedAt:   time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", orderID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "corr-fail-202"

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	// Verify payment record
	payment, err := repo.GetPaymentByOrderID(context.Background(), orderID)
	if err != nil {
		t.Fatalf("failed to get payment for order: %v", err)
	}
	if payment.Status != model.PaymentStatusFailed {
		t.Errorf("expected payment status FAILED, got: %s", payment.Status)
	}

	// Verify outbox has PaymentFailed
	repo.mu.Lock()
	defer repo.mu.Unlock()
	foundFailed := false
	for _, rec := range repo.outboxEvents {
		if rec.EventType == events.EventTypePaymentFailed && rec.Topic == events.TopicPaymentFailed {
			foundFailed = true
			break
		}
	}
	if !foundFailed {
		t.Errorf("expected outbox event PaymentFailed, got: %+v", repo.outboxEvents)
	}
}

func TestPaymentEventConsumer_DuplicateEvent_Idempotent(t *testing.T) {
	c, repo := setupConsumerTest()

	orderID := "ord-idemp-303"
	payload := events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      "usr-idemp-303",
		TotalAmount: 180.00,
		Currency:    "USD",
		CreatedAt:   time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", orderID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "corr-idemp-303"

	// First event invocation
	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("first HandleEvent failed: %v", err)
	}

	repo.mu.Lock()
	initialOutboxCount := len(repo.outboxEvents)
	repo.mu.Unlock()

	// Duplicate event invocation
	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("duplicate HandleEvent failed: %v", err)
	}

	// Outbox count should not increase on duplicate event
	repo.mu.Lock()
	finalOutboxCount := len(repo.outboxEvents)
	repo.mu.Unlock()

	if finalOutboxCount != initialOutboxCount {
		t.Errorf("expected outbox count %d after duplicate, got %d", initialOutboxCount, finalOutboxCount)
	}
}

func TestPaymentEventConsumer_EmptyOrderID_UsesAggregateID(t *testing.T) {
	c, repo := setupConsumerTest()

	aggregateID := "ord-fallback-404"
	payload := events.OrderCreatedEvent{
		OrderID:     "", // empty order ID in payload
		UserID:      "usr-fallback-404",
		TotalAmount: 75.00,
		Currency:    "USD",
		CreatedAt:   time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", aggregateID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	payment, err := repo.GetPaymentByOrderID(context.Background(), aggregateID)
	if err != nil {
		t.Fatalf("failed to retrieve payment by fallback aggregate ID: %v", err)
	}
	if payment.OrderID != aggregateID {
		t.Errorf("expected order ID %s, got %s", aggregateID, payment.OrderID)
	}
}

type contextCheckService struct {
	t             *testing.T
	expectedCorr  string
	handledCalled bool
}

func (s *contextCheckService) HandleOrderCreated(ctx context.Context, event *events.OrderCreatedEvent, correlationID, traceID string) (*model.Payment, error) {
	s.handledCalled = true
	corrVal, _ := ctx.Value(logger.CorrelationIDKey).(string)
	if corrVal != s.expectedCorr {
		s.t.Errorf("expected correlation ID %s in context, got %s", s.expectedCorr, corrVal)
	}
	reqVal, _ := ctx.Value(logger.RequestIDKey).(string)
	if reqVal != s.expectedCorr {
		s.t.Errorf("expected request ID %s in context, got %s", s.expectedCorr, reqVal)
	}
	return nil, nil
}

func (s *contextCheckService) ProcessPayment(ctx context.Context, dto *service.ProcessPaymentDTO) (*model.Payment, error) {
	return nil, nil
}
func (s *contextCheckService) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	return nil, nil
}
func (s *contextCheckService) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	return nil, nil
}
func (s *contextCheckService) CreateRefund(ctx context.Context, dto *service.CreateRefundDTO) (*model.Refund, error) {
	return nil, nil
}
func (s *contextCheckService) CancelPayment(ctx context.Context, dto *service.CancelPaymentDTO) (*model.Payment, error) {
	return nil, nil
}
func (s *contextCheckService) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
	return nil, nil
}
func (s *contextCheckService) HandleRazorpayWebhook(ctx context.Context, body []byte) error {
	return nil
}

func TestPaymentEventConsumer_PropagatesCorrelationID(t *testing.T) {
	mockSvc := &contextCheckService{
		t:            t,
		expectedCorr: "test-corr-xyz",
	}
	c := consumer.NewPaymentEventConsumer(mockSvc)

	payload := events.OrderCreatedEvent{
		OrderID: "ord-test-corr",
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeOrderCreated, "order-service", payload.OrderID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "test-corr-xyz"

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	if !mockSvc.handledCalled {
		t.Error("expected HandleOrderCreated to be called")
	}
}
