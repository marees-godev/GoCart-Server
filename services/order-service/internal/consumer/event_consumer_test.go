package consumer_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/consumer"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/order-service/internal/service"
)

type mockConsumerRepo struct {
	orders          map[string]*model.Order
	outboxEnvelopes []*events.EventEnvelope
	outboxTopic     string
}

func newMockConsumerRepo() *mockConsumerRepo {
	return &mockConsumerRepo{
		orders:          make(map[string]*model.Order),
		outboxEnvelopes: make([]*events.EventEnvelope, 0),
	}
}

func (m *mockConsumerRepo) CreateOrdersWithOutbox(ctx context.Context, orders []*model.Order, envelopes []*events.EventEnvelope, topic string) error {
	for _, o := range orders {
		m.orders[o.ID] = o
	}
	m.outboxEnvelopes = append(m.outboxEnvelopes, envelopes...)
	m.outboxTopic = topic
	return nil
}

func (m *mockConsumerRepo) GetOrderByID(ctx context.Context, id string) (*model.Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, repository.ErrOrderNotFound
	}
	cp := *o
	return &cp, nil
}

func (m *mockConsumerRepo) GetOrdersByIdempotencyKey(ctx context.Context, idempotencyKey string) ([]*model.Order, error) {
	return nil, nil
}

func (m *mockConsumerRepo) ListOrdersByUserID(ctx context.Context, userID string, limit, offset int) ([]*model.Order, int, error) {
	return nil, 0, nil
}

func (m *mockConsumerRepo) UpdateOrderStatus(ctx context.Context, id string, status model.OrderStatus, reason ...string) (*model.Order, error) {
	return m.UpdateOrderStatusWithOutbox(ctx, id, status, nil, "", reason...)
}

func (m *mockConsumerRepo) UpdateOrderStatusWithOutbox(ctx context.Context, id string, status model.OrderStatus, envelope *events.EventEnvelope, topic string, reason ...string) (*model.Order, error) {
	o, ok := m.orders[id]
	if !ok {
		return nil, repository.ErrOrderNotFound
	}
	if o.Status == status {
		cp := *o
		return &cp, nil
	}
	o.Status = status
	switch status {
	case model.OrderStatusConfirmed, model.OrderStatusPaid:
		o.PaymentStatus = model.PaymentStatusPaid
	case model.OrderStatusPaymentFailed:
		o.PaymentStatus = model.PaymentStatusFailed
	}
	o.UpdatedAt = time.Now().UTC()

	if envelope != nil {
		m.outboxEnvelopes = append(m.outboxEnvelopes, envelope)
		m.outboxTopic = topic
	}
	cp := *o
	return &cp, nil
}

func (m *mockConsumerRepo) CancelOrder(ctx context.Context, id string, envelope *events.EventEnvelope, topic string, reason ...string) (*model.Order, error) {
	return nil, nil
}

func (m *mockConsumerRepo) GetOrderStatusHistory(ctx context.Context, orderID string) ([]*model.OrderStatusHistory, error) {
	return nil, nil
}

func (m *mockConsumerRepo) CreateOrderWithOutbox(ctx context.Context, orderID string, insertOrder func(ctx context.Context, tx pgx.Tx) error, envelope *events.EventEnvelope, topic string) error {
	return nil
}

func setupConsumerTest() (*consumer.OrderEventConsumer, *mockConsumerRepo, service.OrderService) {
	repo := newMockConsumerRepo()
	svc := service.NewOrderService(repo, nil, nil, nil, 5, nil)
	c := consumer.NewOrderEventConsumer(svc)
	return c, repo, svc
}

func TestOrderEventConsumer_PaymentSuccessful_ConfirmsOrder(t *testing.T) {
	c, repo, _ := setupConsumerTest()
	orderID := uuid.New().String()
	repo.orders[orderID] = &model.Order{
		ID:            orderID,
		UserID:        uuid.New().String(),
		Status:        model.OrderStatusCreated,
		PaymentStatus: model.PaymentStatusPending,
		TotalAmount:   150.00,
		Currency:      "USD",
	}

	payload := events.PaymentSuccessfulEvent{
		PaymentID:     uuid.New().String(),
		OrderID:       orderID,
		UserID:        repo.orders[orderID].UserID,
		Amount:        150.00,
		Currency:      "USD",
		PaymentMethod: "CREDIT_CARD",
		PaidAt:        time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentSuccessful, "payment-service", payload.PaymentID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "corr-test-123"

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	order := repo.orders[orderID]
	if order.Status != model.OrderStatusConfirmed {
		t.Errorf("expected order status CONFIRMED, got %s", order.Status)
	}
	if order.PaymentStatus != model.PaymentStatusPaid {
		t.Errorf("expected payment status PAID, got %s", order.PaymentStatus)
	}

	// Verify outbox has OrderConfirmed event
	if len(repo.outboxEnvelopes) != 1 {
		t.Fatalf("expected 1 outbox envelope, got %d", len(repo.outboxEnvelopes))
	}
	outboxEnv := repo.outboxEnvelopes[0]
	if outboxEnv.EventType != events.EventTypeOrderConfirmed {
		t.Errorf("expected outbox event type OrderConfirmed, got %s", outboxEnv.EventType)
	}
	if outboxEnv.CorrelationID != "corr-test-123" {
		t.Errorf("expected correlation ID corr-test-123, got %s", outboxEnv.CorrelationID)
	}
	if repo.outboxTopic != events.TopicOrderConfirmed {
		t.Errorf("expected topic %s, got %s", events.TopicOrderConfirmed, repo.outboxTopic)
	}

	// Test Idempotency: duplicate event does not recreate or duplicate state changes
	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("duplicate HandleEvent failed: %v", err)
	}
	if len(repo.outboxEnvelopes) != 1 {
		t.Errorf("expected outbox envelopes to remain 1 after duplicate event, got %d", len(repo.outboxEnvelopes))
	}
}

func TestOrderEventConsumer_PaymentFailed_UpdatesOrderStatus(t *testing.T) {
	c, repo, _ := setupConsumerTest()
	orderID := uuid.New().String()
	repo.orders[orderID] = &model.Order{
		ID:            orderID,
		UserID:        uuid.New().String(),
		Status:        model.OrderStatusCreated,
		PaymentStatus: model.PaymentStatusPending,
		TotalAmount:   200.00,
		Currency:      "USD",
	}

	payload := events.PaymentFailedEvent{
		PaymentID: uuid.New().String(),
		OrderID:   orderID,
		UserID:    repo.orders[orderID].UserID,
		Amount:    200.00,
		Reason:    "insufficient funds",
		FailedAt:  time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentFailed, "payment-service", payload.PaymentID, payload)
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}
	env.CorrelationID = "corr-fail-456"

	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("HandleEvent failed: %v", err)
	}

	order := repo.orders[orderID]
	if order.Status != model.OrderStatusPaymentFailed {
		t.Errorf("expected order status PAYMENT_FAILED, got %s", order.Status)
	}
	if order.PaymentStatus != model.PaymentStatusFailed {
		t.Errorf("expected payment status FAILED, got %s", order.PaymentStatus)
	}

	// Verify outbox envelope recorded OrderPaymentFailed event
	if len(repo.outboxEnvelopes) != 1 {
		t.Fatalf("expected 1 outbox envelope, got %d", len(repo.outboxEnvelopes))
	}
	outboxEnv := repo.outboxEnvelopes[0]
	if outboxEnv.EventType != events.EventTypeOrderPaymentFailed {
		t.Errorf("expected outbox event_type %s, got %s", events.EventTypeOrderPaymentFailed, outboxEnv.EventType)
	}
	if repo.outboxTopic != events.TopicOrderPaymentFailed {
		t.Errorf("expected outbox topic %s, got %s", events.TopicOrderPaymentFailed, repo.outboxTopic)
	}

	// Test Idempotency: duplicate event does not error and outbox is not duplicated
	if err := c.HandleEvent(context.Background(), env); err != nil {
		t.Fatalf("duplicate PaymentFailed HandleEvent failed: %v", err)
	}
	if order.Status != model.OrderStatusPaymentFailed {
		t.Errorf("expected order status to remain PAYMENT_FAILED, got %s", order.Status)
	}
	if len(repo.outboxEnvelopes) != 1 {
		t.Errorf("expected outbox envelopes to remain 1 after duplicate, got %d", len(repo.outboxEnvelopes))
	}
}

func TestOrderEventConsumer_MalformedPayload_ReturnsError(t *testing.T) {
	c, _, _ := setupConsumerTest()
	env := &events.EventEnvelope{
		EventType: events.EventTypePaymentSuccessful,
		Data:      []byte(`{invalid-json`),
	}

	err := c.HandleEvent(context.Background(), env)
	if err == nil {
		t.Errorf("expected error for malformed payload, got nil")
	}
}

func TestOrderEventConsumer_NilEnvelope_ReturnsError(t *testing.T) {
	c, _, _ := setupConsumerTest()
	err := c.HandleEvent(context.Background(), nil)
	if err == nil {
		t.Errorf("expected error for nil envelope, got nil")
	}
}

func TestOrderEventConsumer_UnknownEvent_IgnoredCleanly(t *testing.T) {
	c, _, _ := setupConsumerTest()
	env := &events.EventEnvelope{
		EventType: "UnknownEvent",
		Data:      []byte(`{}`),
	}

	err := c.HandleEvent(context.Background(), env)
	if err != nil {
		t.Errorf("expected unknown event to be ignored cleanly, got error: %v", err)
	}
}
