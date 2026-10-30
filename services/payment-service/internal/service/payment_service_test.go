package service_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type outboxRecord struct {
	AggregateID string
	EventType   string
	Topic       string
}

// In-Memory Repository Mock
type mockPaymentRepo struct {
	mu           sync.Mutex
	payments     map[string]*model.Payment
	idempMap     map[string]*model.Payment
	orderMap     map[string]*model.Payment
	refunds      map[string]*model.Refund
	processedEvs map[string]bool
	outboxEvents []outboxRecord
	updateCalls  int
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

	m.updateCalls++
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
			exists := false
			for _, rec := range m.outboxEvents {
				if rec.AggregateID == p.ID && rec.EventType == events.EventTypePaymentSuccessful {
					exists = true
					break
				}
			}
			if !exists {
				m.outboxEvents = append(m.outboxEvents, outboxRecord{
					AggregateID: p.ID,
					EventType:   events.EventTypePaymentSuccessful,
					Topic:       events.TopicPaymentSuccessful,
				})
			}
		case model.PaymentStatusFailed:
			exists := false
			for _, rec := range m.outboxEvents {
				if rec.AggregateID == p.ID && rec.EventType == events.EventTypePaymentFailed {
					exists = true
					break
				}
			}
			if !exists {
				m.outboxEvents = append(m.outboxEvents, outboxRecord{
					AggregateID: p.ID,
					EventType:   events.EventTypePaymentFailed,
					Topic:       events.TopicPaymentFailed,
				})
			}
		case model.PaymentStatusCancelled:
			exists := false
			for _, rec := range m.outboxEvents {
				if rec.AggregateID == p.ID && rec.EventType == events.EventTypePaymentCancelled {
					exists = true
					break
				}
			}
			if !exists {
				m.outboxEvents = append(m.outboxEvents, outboxRecord{
					AggregateID: p.ID,
					EventType:   events.EventTypePaymentCancelled,
					Topic:       events.TopicPaymentCancelled,
				})
			}
		}
	}

	m.idempMap[p.IdempotencyKey] = p
	m.orderMap[p.OrderID] = p

	pCopy := *p
	return &pCopy, nil
}

func (m *mockPaymentRepo) CreateRefund(ctx context.Context, ref *model.Refund) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if ref.IdempotencyKey != "" {
		for _, existing := range m.refunds {
			if existing.IdempotencyKey == ref.IdempotencyKey {
				return appErrors.Conflict("duplicate refund idempotency key")
			}
		}
	}

	m.nextID++
	ref.ID = fmt.Sprintf("ref_uuid_%d", m.nextID)
	ref.CreatedAt = time.Now()
	ref.UpdatedAt = time.Now()

	refCopy := *ref
	m.refunds[ref.ID] = &refCopy

	m.outboxEvents = append(m.outboxEvents, outboxRecord{
		AggregateID: ref.ID,
		EventType:   events.EventTypeRefundInitiated,
		Topic:       events.TopicRefundInitiated,
	})

	return nil
}

func (m *mockPaymentRepo) GetRefundByID(ctx context.Context, id string) (*model.Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	r, ok := m.refunds[id]
	if !ok {
		return nil, appErrors.NotFound("refund not found")
	}
	rCopy := *r
	return &rCopy, nil
}

func (m *mockPaymentRepo) GetRefundByIdempotencyKey(ctx context.Context, key string) (*model.Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, r := range m.refunds {
		if r.IdempotencyKey == key {
			rCopy := *r
			return &rCopy, nil
		}
	}
	return nil, appErrors.NotFound("refund not found")
}

func (m *mockPaymentRepo) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var res []*model.Refund
	for _, r := range m.refunds {
		if r.PaymentID == paymentID {
			rCopy := *r
			res = append(res, &rCopy)
		}
	}
	return res, nil
}

func (m *mockPaymentRepo) GetTotalRefundedAmount(ctx context.Context, paymentID string) (float64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	var total float64
	for _, r := range m.refunds {
		if r.PaymentID == paymentID && r.Status == model.RefundStatusSuccess {
			total += r.Amount
		}
	}
	return total, nil
}

func (m *mockPaymentRepo) SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error {
	return nil
}

func (m *mockPaymentRepo) IsWebhookProcessed(ctx context.Context, eventID string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.processedEvs[eventID], nil
}

func (m *mockPaymentRepo) MarkWebhookProcessed(ctx context.Context, eventID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.processedEvs[eventID] = true
	return nil
}

func (m *mockPaymentRepo) GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, r := range m.refunds {
		if r.GatewayRefundID == gatewayRefundID {
			rCopy := *r
			return &rCopy, nil
		}
	}
	return nil, appErrors.NotFound("refund not found")
}

func (m *mockPaymentRepo) UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.refunds[id]
	if !ok {
		return nil, appErrors.NotFound("refund not found")
	}
	r.Status = status
	if gatewayRefundID != "" {
		r.GatewayRefundID = gatewayRefundID
	}
	r.UpdatedAt = time.Now()

	switch status {
	case model.RefundStatusSuccess:
		m.outboxEvents = append(m.outboxEvents, outboxRecord{
			AggregateID: r.ID,
			EventType:   events.EventTypeRefundCompleted,
			Topic:       events.TopicRefundCompleted,
		})
	case model.RefundStatusFailed:
		m.outboxEvents = append(m.outboxEvents, outboxRecord{
			AggregateID: r.ID,
			EventType:   events.EventTypeRefundFailed,
			Topic:       events.TopicRefundFailed,
		})
	}

	rCopy := *r
	return &rCopy, nil
}

// In-Memory Order Client Mock
type mockOrderClient struct {
	orders map[string]*client.AuthoritativeOrder
}

func newMockOrderClient() *mockOrderClient {
	return &mockOrderClient{
		orders: make(map[string]*client.AuthoritativeOrder),
	}
}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID string) (*client.AuthoritativeOrder, error) {
	ord, ok := m.orders[orderID]
	if !ok {
		return nil, errors.New("order not found")
	}
	return ord, nil
}

// --- Unit Tests ---

func TestProcessPayment_AuthoritativeAmountEnforced(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_authoritative_100"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_123",
		TotalAmount: 250.75, // Authoritative Order Amount
		Currency:    "USD",
		Status:      "CREATED",
	}

	dto := &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_123",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_auth_amt_001",
		ClientAmount:   1.00, // Client tries to modify amount to $1.00
		ClientCurrency: "USD",
	}

	payment, err := svc.ProcessPayment(context.Background(), dto)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if payment.Amount != 250.75 {
		t.Errorf("expected payment amount to match authoritative order 250.75, got %.2f", payment.Amount)
	}

	if payment.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status SUCCESS, got %s", payment.Status)
	}

	if payment.GatewayTransactionID == "" {
		t.Errorf("expected non-empty gateway transaction ID")
	}
}

func TestProcessPayment_AuthoritativeOrderUnavailable_RejectsPayment(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	// Do NOT register the order in orderClient to simulate order service error / order not found
	orderID := "ord_unavailable_404"

	dto := &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_123",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_unavail_001",
		ClientAmount:   500.00, // Client tries to supply amount as fallback
		ClientCurrency: "USD",
	}

	payment, err := svc.ProcessPayment(context.Background(), dto)
	if err == nil {
		t.Fatalf("expected error when authoritative order cannot be fetched, got payment: %+v", payment)
	}

	// Verify payment was NOT created in repository
	existing, _ := repo.GetPaymentByIdempotencyKey(context.Background(), dto.IdempotencyKey)
	if existing != nil {
		t.Errorf("expected no payment record to be created on authoritative order fetch failure")
	}
}

func TestProcessPayment_NilOrderClient_RejectsPayment(t *testing.T) {
	repo := newMockPaymentRepo()
	gw := gateway.NewMockGateway(2 * time.Second)
	// Order client is nil
	svc := service.NewPaymentService(repo, gw, nil)

	dto := &service.ProcessPaymentDTO{
		OrderID:        "ord_any_123",
		UserID:         "usr_123",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_nil_client_001",
		ClientAmount:   100.00,
		ClientCurrency: "USD",
	}

	payment, err := svc.ProcessPayment(context.Background(), dto)
	if err == nil {
		t.Fatalf("expected error when orderClient is nil, got payment: %+v", payment)
	}
}

func TestHandleOrderCreated_OrderWithFailSubstring_SucceedsNormally(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord-fail-legitimate-id-555"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_555",
		TotalAmount: 75.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	event := &events.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      "usr_555",
		TotalAmount: 75.00,
		Currency:    "USD",
	}

	// Correlation ID also contains "fail"
	payment, err := svc.HandleOrderCreated(context.Background(), event, "corr-fail-check", "trace-123")
	if err != nil {
		t.Fatalf("expected payment to succeed for order with 'fail' substring, got error: %v", err)
	}

	if payment.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status SUCCESS, got: %s (failure_reason: %s)", payment.Status, payment.FailureReason)
	}

	// Verify idempotency key is clean without '-fail_gateway' suffix
	expectedIdempKey := "order-pay-" + orderID
	if payment.IdempotencyKey != expectedIdempKey {
		t.Errorf("expected idempotency key %s, got %s", expectedIdempKey, payment.IdempotencyKey)
	}
}

func TestProcessPayment_IdempotencyKey_PreventsDuplicateCharges(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_idemp_200"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_456",
		TotalAmount: 120.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	idempKey := "idemp_unique_key_999"
	dto := &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_456",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: idempKey,
	}

	payment1, err := svc.ProcessPayment(context.Background(), dto)
	if err != nil {
		t.Fatalf("first payment call failed: %v", err)
	}

	// Immediate duplicate payment request with identical idempotency key
	payment2, err := svc.ProcessPayment(context.Background(), dto)
	if err != nil {
		t.Fatalf("duplicate payment call failed: %v", err)
	}

	if payment1.ID != payment2.ID {
		t.Errorf("expected same payment object for duplicate idempotency key, got IDs %s and %s", payment1.ID, payment2.ID)
	}

	if payment2.GatewayTransactionID != payment1.GatewayTransactionID {
		t.Errorf("expected same gateway transaction ID, got %s and %s", payment1.GatewayTransactionID, payment2.GatewayTransactionID)
	}
}

func TestProcessPayment_GatewayFailureHandledSanitized(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_fail_300"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_789",
		TotalAmount: 89.90,
		Currency:    "USD",
		Status:      "CREATED",
	}

	dto := &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_789",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_fail_gateway_test",
	}

	payment, err := svc.ProcessPayment(context.Background(), dto)
	if err != nil {
		t.Fatalf("expected service to return failed payment struct without crash, got err: %v", err)
	}

	if payment.Status != model.PaymentStatusFailed {
		t.Errorf("expected payment status FAILED, got %s", payment.Status)
	}

	if payment.FailureReason == "" {
		t.Errorf("expected failure reason to be stored")
	}
}

func TestCreateRefund_SuccessAndValidation(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_refund_400"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_ref",
		TotalAmount: 50.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_ref",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_ref_pay_1",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to setup successful payment for refund test: %v", err)
	}

	// 1. Attempt refund exceeding payment amount
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         100.00, // Exceeds $50.00
		Reason:         "too expensive",
		IdempotencyKey: "ref_idemp_over_amount",
	})
	if err == nil {
		t.Fatalf("expected error when refund exceeds payment amount, got nil")
	}

	// 2. Attempt valid refund
	refund, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "customer request",
		IdempotencyKey: "ref_idemp_valid_001",
	})
	if err != nil {
		t.Fatalf("expected valid refund to succeed, got %v", err)
	}

	if refund.Status != model.RefundStatusSuccess {
		t.Errorf("expected refund status SUCCESS, got %s", refund.Status)
	}
	if refund.GatewayRefundID == "" {
		t.Errorf("expected non-empty gateway refund ID")
	}
}

func TestHandleRazorpayWebhook_Success(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_wh_success_100"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_wh_1",
		TotalAmount: 150.00,
		Currency:    "INR",
		Status:      "CREATED",
	}

	// Create initial payment
	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_wh_1",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_wh_success_1",
	})
	if err != nil {
		t.Fatalf("failed to process initial payment: %v", err)
	}

	// Reset status back to PENDING to test webhook status transition
	repo.payments[payment.ID].Status = model.PaymentStatusPending

	webhookPayload := []byte(`{
		"entity": "event",
		"account_id": "acc_test123",
		"event": "payment.captured",
		"contains": ["payment"],
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_captured_999",
					"entity": "payment",
					"amount": 15000,
					"currency": "INR",
					"status": "captured",
					"order_id": "order_rzp_ord_100",
					"captured": true,
					"notes": {
						"payment_id": "` + payment.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), webhookPayload); err != nil {
		t.Fatalf("expected successful webhook processing, got err: %v", err)
	}

	updated, err := repo.GetPaymentByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to retrieve updated payment: %v", err)
	}

	if updated.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status SUCCESS, got %s", updated.Status)
	}

	if updated.GatewayTransactionID != "pay_rzp_captured_999" {
		t.Errorf("expected gateway transaction ID pay_rzp_captured_999, got %s", updated.GatewayTransactionID)
	}
}

func TestHandleRazorpayWebhook_Failed(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_wh_fail_200"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_wh_2",
		TotalAmount: 200.00,
		Currency:    "INR",
		Status:      "CREATED",
	}

	// Create initial payment
	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_wh_2",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_wh_fail_1",
	})
	if err != nil {
		t.Fatalf("failed to process initial payment: %v", err)
	}

	// Set status to PENDING
	repo.payments[payment.ID].Status = model.PaymentStatusPending

	webhookPayload := []byte(`{
		"entity": "event",
		"account_id": "acc_test123",
		"event": "payment.failed",
		"contains": ["payment"],
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_failed_888",
					"entity": "payment",
					"amount": 20000,
					"currency": "INR",
					"status": "failed",
					"order_id": "order_rzp_ord_200",
					"error_code": "BAD_REQUEST_ERROR",
					"error_description": "Card expired",
					"notes": {
						"payment_id": "` + payment.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), webhookPayload); err != nil {
		t.Fatalf("expected successful webhook processing, got err: %v", err)
	}

	updated, err := repo.GetPaymentByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to retrieve updated payment: %v", err)
	}

	if updated.Status != model.PaymentStatusFailed {
		t.Errorf("expected payment status FAILED, got %s", updated.Status)
	}

	if updated.FailureReason != "Card expired" {
		t.Errorf("expected failure reason 'Card expired', got %s", updated.FailureReason)
	}
}

func TestProcessPayment_NetBanking_NormalizedAndUUID(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_netbanking_500"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_nb_1",
		TotalAmount: 350.00,
		Currency:    "INR",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_nb_1",
		PaymentMethod:  "netbanking",
		IdempotencyKey: "idemp_netbanking_test_100",
	})
	if err != nil {
		t.Fatalf("expected successful payment processing for netbanking, got err: %v", err)
	}

	if payment.PaymentMethod != "NET_BANKING" {
		t.Errorf("expected normalized PaymentMethod 'NET_BANKING', got '%s'", payment.PaymentMethod)
	}

	if payment.PaymentMethodID == nil || *payment.PaymentMethodID == "" {
		t.Errorf("expected non-nil UUID for PaymentMethodID, got nil/empty")
	}
}

func TestHandleRazorpayWebhook_DuplicateSuccess_PublishedOnce(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID: "ord_dup_succ",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	payload := []byte(`{
		"id": "evt_captured_100",
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_dup_succ",
					"notes": {
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	// First event processing
	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("first webhook failed: %v", err)
	}

	p1, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p1.Status != model.PaymentStatusSuccess {
		t.Fatalf("expected SUCCESS, got %s", p1.Status)
	}

	successEventsCount := 0
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentSuccessful {
			successEventsCount++
		}
	}
	if successEventsCount != 1 {
		t.Fatalf("expected PaymentSuccessful outbox event count 1, got %d", successEventsCount)
	}

	callsAfterFirst := repo.updateCalls

	// Second duplicate event processing
	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("duplicate webhook failed: %v", err)
	}

	p2, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p2.Status != model.PaymentStatusSuccess {
		t.Fatalf("expected SUCCESS after duplicate, got %s", p2.Status)
	}

	// Ensure no new update calls or outbox events occurred
	if repo.updateCalls != callsAfterFirst {
		t.Errorf("expected no additional update calls on duplicate event, had %d, now %d", callsAfterFirst, repo.updateCalls)
	}

	successEventsCountAfter := 0
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentSuccessful {
			successEventsCountAfter++
		}
	}
	if successEventsCountAfter != 1 {
		t.Errorf("expected PaymentSuccessful outbox event count to remain 1, got %d", successEventsCountAfter)
	}
}

func TestHandleRazorpayWebhook_DuplicateFailed_PublishedOnce(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID: "ord_dup_fail",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	payload := []byte(`{
		"id": "evt_failed_200",
		"entity": "event",
		"event": "payment.failed",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_dup_fail",
					"error_description": "Insufficient funds",
					"notes": {
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	// First event processing
	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("first webhook failed: %v", err)
	}

	p1, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p1.Status != model.PaymentStatusFailed {
		t.Fatalf("expected FAILED, got %s", p1.Status)
	}

	failedEventsCount := 0
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentFailed {
			failedEventsCount++
		}
	}
	if failedEventsCount != 1 {
		t.Fatalf("expected PaymentFailed outbox event count 1, got %d", failedEventsCount)
	}

	callsAfterFirst := repo.updateCalls

	// Second duplicate event processing
	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("duplicate webhook failed: %v", err)
	}

	p2, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p2.Status != model.PaymentStatusFailed {
		t.Fatalf("expected FAILED after duplicate, got %s", p2.Status)
	}

	if repo.updateCalls != callsAfterFirst {
		t.Errorf("expected no additional update calls on duplicate event, had %d, now %d", callsAfterFirst, repo.updateCalls)
	}

	failedEventsCountAfter := 0
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentFailed {
			failedEventsCountAfter++
		}
	}
	if failedEventsCountAfter != 1 {
		t.Errorf("expected PaymentFailed outbox event count to remain 1, got %d", failedEventsCountAfter)
	}
}

func TestHandleRazorpayWebhook_StateMachine_SuccessCannotTransitionToFailed(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID: "ord_succ_to_fail",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)
	repo.payments[pmt.ID].Status = model.PaymentStatusSuccess

	// Simulate incoming payment.failed event for already successful payment
	payload := []byte(`{
		"id": "evt_conflict_300",
		"entity": "event",
		"event": "payment.failed",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_conflict",
					"error_description": "Network timeout",
					"notes": {
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("expected webhook to be safely handled, got error: %v", err)
	}

	p, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p.Status != model.PaymentStatusSuccess {
		t.Errorf("expected status to remain SUCCESS according to state machine, got %s", p.Status)
	}

	// Verify PaymentFailed was never published
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentFailed {
			t.Errorf("unexpected PaymentFailed event published for successful payment")
		}
	}
}

func TestHandleRazorpayWebhook_StateMachine_FailedCannotTransitionToSuccess(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID: "ord_fail_to_succ",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)
	repo.payments[pmt.ID].Status = model.PaymentStatusFailed

	// Simulate incoming payment.captured event for already failed payment
	payload := []byte(`{
		"id": "evt_conflict_400",
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_conflict_2",
					"notes": {
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), payload); err != nil {
		t.Fatalf("expected webhook to be safely handled, got error: %v", err)
	}

	p, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if p.Status != model.PaymentStatusFailed {
		t.Errorf("expected status to remain FAILED according to state machine, got %s", p.Status)
	}

	// Verify PaymentSuccessful was never published
	for _, evt := range repo.outboxEvents {
		if evt.AggregateID == pmt.ID && evt.EventType == events.EventTypePaymentSuccessful {
			t.Errorf("unexpected PaymentSuccessful event published for failed payment")
		}
	}
}

func TestHandleRazorpayWebhook_UnknownPayment_HandledSafely(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	payload := []byte(`{
		"id": "evt_unknown_500",
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_unknown_none",
					"notes": {
						"payment_id": "non_existent_payment_id"
					}
				}
			}
		}
	}`)

	err := svc.HandleRazorpayWebhook(context.Background(), payload)
	if err == nil {
		t.Fatalf("expected error for unknown payment, got nil")
	}

	if !appErrors.IsNotFound(err) {
		t.Errorf("expected NotFound error for unknown payment, got %v", err)
	}

	if len(repo.payments) != 0 {
		t.Errorf("expected no fake payments auto-created, found %d", len(repo.payments))
	}
	if len(repo.outboxEvents) != 0 {
		t.Errorf("expected no outbox events created for unknown payment, found %d", len(repo.outboxEvents))
	}
}

func TestHandleRazorpayWebhook_EmptyBody_Rejected(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	err := svc.HandleRazorpayWebhook(context.Background(), []byte{})
	if err == nil {
		t.Fatalf("expected error for empty body, got nil")
	}
}

func TestHandleRazorpayWebhook_MalformedJSON_Rejected(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	err := svc.HandleRazorpayWebhook(context.Background(), []byte(`{invalid_json`))
	if err == nil {
		t.Fatalf("expected error for malformed json, got nil")
	}
}

func TestCancelPayment_PermittedStates(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	// Case 1: Cancel from INITIATED
	pmtInit := &model.Payment{
		OrderID:        "ord_cancel_init",
		UserID:         "usr_1",
		Amount:         50.00,
		Currency:       "USD",
		Status:         model.PaymentStatusInitiated,
		IdempotencyKey: "idemp_cancel_init",
	}
	_ = repo.CreatePayment(context.Background(), pmtInit)

	cancelledInit, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		PaymentID: pmtInit.ID,
		Reason:    "Customer changed mind",
	})
	if err != nil {
		t.Fatalf("expected successful cancellation from INITIATED, got: %v", err)
	}
	if cancelledInit.Status != model.PaymentStatusCancelled {
		t.Errorf("expected status CANCELLED, got: %s", cancelledInit.Status)
	}
	if cancelledInit.FailureReason != "Customer changed mind" {
		t.Errorf("expected reason 'Customer changed mind', got: %s", cancelledInit.FailureReason)
	}

	// Verify PaymentCancelled event was published to outbox
	foundCancelInitEvt := false
	for _, rec := range repo.outboxEvents {
		if rec.AggregateID == pmtInit.ID && rec.EventType == events.EventTypePaymentCancelled && rec.Topic == events.TopicPaymentCancelled {
			foundCancelInitEvt = true
			break
		}
	}
	if !foundCancelInitEvt {
		t.Errorf("expected PaymentCancelled event in outbox for INITIATED cancellation")
	}

	// Case 2: Cancel from PENDING
	pmtPending := &model.Payment{
		OrderID:        "ord_cancel_pending",
		UserID:         "usr_2",
		Amount:         75.00,
		Currency:       "USD",
		Status:         model.PaymentStatusPending,
		IdempotencyKey: "idemp_cancel_pending",
	}
	_ = repo.CreatePayment(context.Background(), pmtPending)

	cancelledPending, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		PaymentID: pmtPending.ID,
		Reason:    "User cancelled during checkout",
	})
	if err != nil {
		t.Fatalf("expected successful cancellation from PENDING, got: %v", err)
	}
	if cancelledPending.Status != model.PaymentStatusCancelled {
		t.Errorf("expected status CANCELLED, got: %s", cancelledPending.Status)
	}

	foundCancelPendingEvt := false
	for _, rec := range repo.outboxEvents {
		if rec.AggregateID == pmtPending.ID && rec.EventType == events.EventTypePaymentCancelled {
			foundCancelPendingEvt = true
			break
		}
	}
	if !foundCancelPendingEvt {
		t.Errorf("expected PaymentCancelled event in outbox for PENDING cancellation")
	}
}

func TestCancelPayment_DisallowedStates(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	disallowedStatuses := []model.PaymentStatus{
		model.PaymentStatusSuccess,
		model.PaymentStatusFailed,
		model.PaymentStatusRefunded,
		model.PaymentStatusRefundPending,
		model.PaymentStatusPartiallyRefunded,
	}

	for _, st := range disallowedStatuses {
		p := &model.Payment{
			OrderID:        "ord_disallowed_" + string(st),
			UserID:         "usr_test",
			Amount:         100.00,
			Currency:       "USD",
			Status:         st,
			IdempotencyKey: "idemp_disallowed_" + string(st),
		}
		_ = repo.CreatePayment(context.Background(), p)

		_, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
			PaymentID: p.ID,
		})
		if err == nil {
			t.Errorf("expected error cancelling payment with status %s, got nil", st)
		}
	}
}

func TestCancelPayment_Idempotent(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	p := &model.Payment{
		OrderID:        "ord_idemp_cancel",
		UserID:         "usr_idemp",
		Amount:         60.00,
		Currency:       "USD",
		Status:         model.PaymentStatusPending,
		IdempotencyKey: "idemp_cancel_key",
	}
	_ = repo.CreatePayment(context.Background(), p)

	c1, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		PaymentID: p.ID,
		Reason:    "First cancel call",
	})
	if err != nil || c1.Status != model.PaymentStatusCancelled {
		t.Fatalf("first cancel call failed: %v", err)
	}

	eventsBefore := len(repo.outboxEvents)

	// Second cancel call on already CANCELLED payment
	c2, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		PaymentID: p.ID,
		Reason:    "Duplicate cancel call",
	})
	if err != nil {
		t.Fatalf("expected duplicate cancel call to succeed idempotently, got: %v", err)
	}
	if c2.ID != c1.ID || c2.Status != model.PaymentStatusCancelled {
		t.Errorf("expected same cancelled payment, got status: %s", c2.Status)
	}

	// Verify no new outbox events inserted on duplicate call
	if len(repo.outboxEvents) != eventsBefore {
		t.Errorf("expected outbox event count to remain %d, got %d", eventsBefore, len(repo.outboxEvents))
	}
}

func TestCancelPayment_ByOrderID(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	p := &model.Payment{
		OrderID:        "ord_lookup_cancel_1",
		UserID:         "usr_ord_cancel",
		Amount:         45.00,
		Currency:       "USD",
		Status:         model.PaymentStatusPending,
		IdempotencyKey: "idemp_by_ord_id",
	}
	_ = repo.CreatePayment(context.Background(), p)

	cancelled, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		OrderID: "ord_lookup_cancel_1",
		Reason:  "Cancelled via Order ID",
	})
	if err != nil {
		t.Fatalf("expected cancellation by order ID to succeed, got: %v", err)
	}
	if cancelled.ID != p.ID || cancelled.Status != model.PaymentStatusCancelled {
		t.Errorf("expected cancelled payment for order, got status %s", cancelled.Status)
	}
}

func TestCancelPayment_Validation(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	// Missing both PaymentID and OrderID
	_, err := svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{})
	if err == nil {
		t.Errorf("expected error for empty identifiers, got nil")
	}

	// Non-existent payment
	_, err = svc.CancelPayment(context.Background(), &service.CancelPaymentDTO{
		PaymentID: "non_existent_payment_id",
	})
	if err == nil {
		t.Errorf("expected NotFound error for non-existent payment, got nil")
	}
}

func TestCreateRefund_FullRefund(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_full_refund_1"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_refund_1",
		TotalAmount: 100.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_refund_1",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_full_ref_proc",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// Request full refund ($100.00)
	refund, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         100.00,
		Reason:         "Customer returned item",
		IdempotencyKey: "ref_idemp_full_1",
	})
	if err != nil {
		t.Fatalf("expected full refund to succeed, got: %v", err)
	}

	// Acceptance Criteria verifications:
	// 1. Full refunds are supported & transition payment to REFUNDED
	// 2. Refund references original payment (PaymentID and OrderID)
	// 3. Refund state is persisted
	// 4. RefundInitiated and RefundCompleted events published
	if refund.Status != model.RefundStatusSuccess {
		t.Errorf("expected refund status SUCCESS, got: %s", refund.Status)
	}
	if refund.PaymentID != payment.ID {
		t.Errorf("expected refund PaymentID %s, got %s", payment.ID, refund.PaymentID)
	}
	if refund.OrderID != payment.OrderID {
		t.Errorf("expected refund OrderID %s, got %s", payment.OrderID, refund.OrderID)
	}
	if refund.Amount != 100.00 {
		t.Errorf("expected refund Amount 100.00, got %.2f", refund.Amount)
	}

	// Check payment status updated to REFUNDED
	updatedPayment, err := repo.GetPaymentByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to get payment: %v", err)
	}
	if updatedPayment.Status != model.PaymentStatusRefunded {
		t.Errorf("expected payment status REFUNDED after full refund, got: %s", updatedPayment.Status)
	}

	// Verify refund persisted in repository
	savedRefunds, err := repo.GetRefundsByPaymentID(context.Background(), payment.ID)
	if err != nil || len(savedRefunds) != 1 {
		t.Fatalf("expected 1 persisted refund, got %d (err: %v)", len(savedRefunds), err)
	}
	if savedRefunds[0].ID != refund.ID {
		t.Errorf("expected saved refund ID %s, got %s", refund.ID, savedRefunds[0].ID)
	}

	// Verify events in outbox: RefundInitiated and RefundCompleted
	var foundInitiated, foundCompleted bool
	for _, rec := range repo.outboxEvents {
		if rec.AggregateID == refund.ID && rec.EventType == events.EventTypeRefundInitiated && rec.Topic == events.TopicRefundInitiated {
			foundInitiated = true
		}
		if rec.AggregateID == refund.ID && rec.EventType == events.EventTypeRefundCompleted && rec.Topic == events.TopicRefundCompleted {
			foundCompleted = true
		}
	}
	if !foundInitiated {
		t.Errorf("expected RefundInitiated event in outbox for refund %s", refund.ID)
	}
	if !foundCompleted {
		t.Errorf("expected RefundCompleted event in outbox for refund %s", refund.ID)
	}
}

func TestCreateRefund_PartialRefund(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_partial_refund_1"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_partial_1",
		TotalAmount: 100.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_partial_1",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_partial_ref_proc",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// Step 1: First partial refund ($40.00)
	ref1, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         40.00,
		Reason:         "Partial return - item 1",
		IdempotencyKey: "ref_idemp_part_1",
	})
	if err != nil {
		t.Fatalf("first partial refund failed: %v", err)
	}
	if ref1.Status != model.RefundStatusSuccess {
		t.Errorf("expected ref1 status SUCCESS, got %s", ref1.Status)
	}

	p1, _ := repo.GetPaymentByID(context.Background(), payment.ID)
	if p1.Status != model.PaymentStatusPartiallyRefunded {
		t.Errorf("expected payment status PARTIALLY_REFUNDED after 1st partial refund, got %s", p1.Status)
	}

	// Step 2: Second partial refund ($35.00)
	ref2, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         35.00,
		Reason:         "Partial return - item 2",
		IdempotencyKey: "ref_idemp_part_2",
	})
	if err != nil {
		t.Fatalf("second partial refund failed: %v", err)
	}
	if ref2.Status != model.RefundStatusSuccess {
		t.Errorf("expected ref2 status SUCCESS, got %s", ref2.Status)
	}

	p2, _ := repo.GetPaymentByID(context.Background(), payment.ID)
	if p2.Status != model.PaymentStatusPartiallyRefunded {
		t.Errorf("expected payment status PARTIALLY_REFUNDED after 2nd partial refund, got %s", p2.Status)
	}

	// Step 3: Third refund completing remaining balance ($25.00)
	ref3, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         25.00,
		Reason:         "Final partial return - item 3",
		IdempotencyKey: "ref_idemp_part_3",
	})
	if err != nil {
		t.Fatalf("third refund failed: %v", err)
	}
	if ref3.Status != model.RefundStatusSuccess {
		t.Errorf("expected ref3 status SUCCESS, got %s", ref3.Status)
	}

	p3, _ := repo.GetPaymentByID(context.Background(), payment.ID)
	if p3.Status != model.PaymentStatusRefunded {
		t.Errorf("expected payment status REFUNDED after cumulative refunds complete total amount, got %s", p3.Status)
	}

	// Verify all 3 refunds persisted and traceable to original payment
	savedRefunds, err := repo.GetRefundsByPaymentID(context.Background(), payment.ID)
	if err != nil || len(savedRefunds) != 3 {
		t.Fatalf("expected 3 persisted refunds, got %d (err: %v)", len(savedRefunds), err)
	}
	for _, r := range savedRefunds {
		if r.PaymentID != payment.ID || r.OrderID != payment.OrderID {
			t.Errorf("refund %s failed traceability check: PaymentID=%s, OrderID=%s", r.ID, r.PaymentID, r.OrderID)
		}
	}
}

func TestCreateRefund_ExceedsRefundableAmount(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_over_refund_1"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_over_1",
		TotalAmount: 100.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_over_1",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_over_ref_proc",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// 1. Initial refund exceeding total payment amount
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         100.01,
		Reason:         "Exceeds initial amount",
		IdempotencyKey: "ref_over_1",
	})
	if err == nil {
		t.Fatalf("expected error when refund exceeds payment amount, got nil")
	}

	// 2. Valid partial refund of $60
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         60.00,
		Reason:         "Valid partial",
		IdempotencyKey: "ref_valid_part_1",
	})
	if err != nil {
		t.Fatalf("valid partial refund failed: %v", err)
	}

	// 3. Subsequent refund of $40.01 exceeds remaining refundable amount ($40.00)
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         40.01,
		Reason:         "Exceeds remaining amount",
		IdempotencyKey: "ref_over_2",
	})
	if err == nil {
		t.Fatalf("expected error when refund exceeds remaining refundable amount, got nil")
	}

	// 4. Zero or negative refund amount
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         0,
		Reason:         "Zero amount",
		IdempotencyKey: "ref_zero",
	})
	if err == nil {
		t.Fatalf("expected error for zero refund amount, got nil")
	}

	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         -10.00,
		Reason:         "Negative amount",
		IdempotencyKey: "ref_neg",
	})
	if err == nil {
		t.Fatalf("expected error for negative refund amount, got nil")
	}

	// 5. Complete remaining $40.00 -> status becomes REFUNDED
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         40.00,
		Reason:         "Complete refund",
		IdempotencyKey: "ref_complete",
	})
	if err != nil {
		t.Fatalf("complete refund failed: %v", err)
	}

	// 6. Attempting another refund on REFUNDED payment should fail
	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         10.00,
		Reason:         "Attempt on fully refunded",
		IdempotencyKey: "ref_after_full",
	})
	if err == nil {
		t.Fatalf("expected error refunding already fully refunded payment, got nil")
	}
}

func TestCreateRefund_DuplicateRefundPrevented(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_dup_refund_1"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_dup_ref",
		TotalAmount: 80.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_dup_ref",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_dup_ref_proc",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	idempKey := "ref_idemp_unique_key_777"
	ref1, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "Return item",
		IdempotencyKey: idempKey,
	})
	if err != nil {
		t.Fatalf("first refund call failed: %v", err)
	}

	outboxCountAfterFirst := len(repo.outboxEvents)

	// Immediate duplicate refund call with identical idempotency key
	ref2, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "Return item duplicate",
		IdempotencyKey: idempKey,
	})
	if err != nil {
		t.Fatalf("duplicate refund call failed: %v", err)
	}

	// Must return the exact same refund
	if ref1.ID != ref2.ID {
		t.Errorf("expected same refund ID on duplicate request, got %s and %s", ref1.ID, ref2.ID)
	}
	if ref2.GatewayRefundID != ref1.GatewayRefundID {
		t.Errorf("expected same gateway refund ID, got %s and %s", ref1.GatewayRefundID, ref2.GatewayRefundID)
	}

	// Ensure no duplicate records created in refunds table
	refundsList, _ := repo.GetRefundsByPaymentID(context.Background(), payment.ID)
	if len(refundsList) != 1 {
		t.Errorf("expected exactly 1 refund record, found %d", len(refundsList))
	}

	// Ensure no duplicate outbox events
	if len(repo.outboxEvents) != outboxCountAfterFirst {
		t.Errorf("expected outbox event count to remain %d, got %d", outboxCountAfterFirst, len(repo.outboxEvents))
	}

	// Attempt using same idempotency key for a different payment ID must return conflict
	pOther := &model.Payment{
		OrderID:        "ord_other_pay",
		UserID:         "usr_dup_ref",
		Amount:         80.00,
		Currency:       "USD",
		Status:         model.PaymentStatusSuccess,
		IdempotencyKey: "idemp_other_pay",
	}
	_ = repo.CreatePayment(context.Background(), pOther)

	_, err = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      pOther.ID,
		Amount:         30.00,
		Reason:         "Conflict attempt",
		IdempotencyKey: idempKey,
	})
	if err == nil {
		t.Errorf("expected Conflict error when idempotency key is used for different payment, got nil")
	}
}

func TestCreateRefund_FailedRefund_SafelyHandled_GatewayDeclined(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_fail_refund_declined"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_fail_gw",
		TotalAmount: 70.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_fail_gw",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_fail_ref_proc_1",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// Trigger simulated gateway decline via "fail_gateway"
	refund, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         20.00,
		Reason:         "Customer return - fail_gateway",
		IdempotencyKey: "ref_idemp_fail_gateway_1",
	})
	if err != nil {
		t.Fatalf("expected graceful return of failed refund struct, got err: %v", err)
	}

	// Acceptance criteria: Failed refunds are handled safely
	// Refund status is FAILED
	if refund.Status != model.RefundStatusFailed {
		t.Errorf("expected refund status FAILED, got: %s", refund.Status)
	}

	// Payment status safely reverts back to SUCCESS (not stuck in REFUND_PENDING)
	updatedPayment, err := repo.GetPaymentByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to get payment: %v", err)
	}
	if updatedPayment.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status to revert to SUCCESS, got: %s", updatedPayment.Status)
	}

	// Outbox event RefundFailed is published
	foundFailedEvt := false
	for _, rec := range repo.outboxEvents {
		if rec.AggregateID == refund.ID && rec.EventType == events.EventTypeRefundFailed && rec.Topic == events.TopicRefundFailed {
			foundFailedEvt = true
			break
		}
	}
	if !foundFailedEvt {
		t.Errorf("expected RefundFailed event in outbox")
	}
}

func TestCreateRefund_FailedRefund_SafelyHandled_GatewayError(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_fail_refund_error"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_err_gw",
		TotalAmount: 90.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_err_gw",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_err_ref_proc_1",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// Trigger simulated gateway error via "error_gateway"
	refund, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "error_gateway",
		IdempotencyKey: "ref_idemp_error_gateway_1",
	})
	if err == nil {
		t.Fatalf("expected error from gateway failure, got nil")
	}
	if refund != nil && refund.Status != model.RefundStatusFailed {
		t.Errorf("expected refund status FAILED, got: %s", refund.Status)
	}

	// Payment status safely reverts back to SUCCESS (not stuck in REFUND_PENDING)
	updatedPayment, err := repo.GetPaymentByID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("failed to get payment: %v", err)
	}
	if updatedPayment.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status to revert to SUCCESS, got: %s", updatedPayment.Status)
	}

	// RefundFailed outbox event was published
	foundFailedEvt := false
	for _, rec := range repo.outboxEvents {
		if rec.AggregateID == refund.ID && rec.EventType == events.EventTypeRefundFailed {
			foundFailedEvt = true
			break
		}
	}
	if !foundFailedEvt {
		t.Errorf("expected RefundFailed event in outbox")
	}
}

func TestCreateRefund_PartialRefund_ThenFailedRefund_SafelyRevertsToPartiallyRefunded(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	orderID := "ord_part_then_fail"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_part_fail",
		TotalAmount: 100.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_part_fail",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_part_fail_proc",
	})
	if err != nil || payment.Status != model.PaymentStatusSuccess {
		t.Fatalf("failed to process payment: %v", err)
	}

	// 1. First partial refund of $40 succeeds
	ref1, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         40.00,
		Reason:         "First partial",
		IdempotencyKey: "ref_part_succ_1",
	})
	if err != nil || ref1.Status != model.RefundStatusSuccess {
		t.Fatalf("first partial refund failed: %v", err)
	}

	p1, _ := repo.GetPaymentByID(context.Background(), payment.ID)
	if p1.Status != model.PaymentStatusPartiallyRefunded {
		t.Fatalf("expected payment PARTIALLY_REFUNDED, got: %s", p1.Status)
	}

	// 2. Second refund of $30 fails at gateway
	ref2, err := svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "Second partial fail_gateway",
		IdempotencyKey: "ref_part_fail_gateway_2",
	})
	if err != nil {
		t.Fatalf("expected graceful return of failed refund, got: %v", err)
	}
	if ref2.Status != model.RefundStatusFailed {
		t.Errorf("expected ref2 status FAILED, got: %s", ref2.Status)
	}

	// Payment status must revert back to PARTIALLY_REFUNDED (NOT SUCCESS and NOT REFUND_PENDING)
	p2, _ := repo.GetPaymentByID(context.Background(), payment.ID)
	if p2.Status != model.PaymentStatusPartiallyRefunded {
		t.Errorf("expected payment status to revert back to PARTIALLY_REFUNDED, got: %s", p2.Status)
	}
}

func TestGetRefundsByPaymentID(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	// Validation: empty paymentID returns error
	_, err := svc.GetRefundsByPaymentID(context.Background(), "")
	if err == nil {
		t.Errorf("expected error for empty paymentID, got nil")
	}

	orderID := "ord_get_refunds"
	orderClient.orders[orderID] = &client.AuthoritativeOrder{
		ID:          orderID,
		UserID:      "usr_get_ref",
		TotalAmount: 100.00,
		Currency:    "USD",
		Status:      "CREATED",
	}

	payment, err := svc.ProcessPayment(context.Background(), &service.ProcessPaymentDTO{
		OrderID:        orderID,
		UserID:         "usr_get_ref",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_get_ref_proc",
	})
	if err != nil {
		t.Fatalf("failed to process payment: %v", err)
	}

	_, _ = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         25.00,
		Reason:         "Refund 1",
		IdempotencyKey: "ref_get_1",
	})
	_, _ = svc.CreateRefund(context.Background(), &service.CreateRefundDTO{
		PaymentID:      payment.ID,
		Amount:         30.00,
		Reason:         "Refund 2",
		IdempotencyKey: "ref_get_2",
	})

	refunds, err := svc.GetRefundsByPaymentID(context.Background(), payment.ID)
	if err != nil {
		t.Fatalf("GetRefundsByPaymentID failed: %v", err)
	}
	if len(refunds) != 2 {
		t.Errorf("expected 2 refunds, got %d", len(refunds))
	}
}

func TestHandleRazorpayWebhook_RefundProcessed_FullAndPartial(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID:              "ord_wh_ref_test",
		Amount:               100.00,
		Currency:             "INR",
		Status:               model.PaymentStatusSuccess,
		GatewayTransactionID: "pay_rzp_orig_100",
		IdempotencyKey:       "idemp_wh_ref_pmt",
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	// Create pending refund in repo
	ref := &model.Refund{
		PaymentID:       pmt.ID,
		OrderID:         pmt.OrderID,
		Amount:          40.00,
		Status:          model.RefundStatusPending,
		GatewayRefundID: "rfnd_rzp_123",
	}
	_ = repo.CreateRefund(context.Background(), ref)

	// Simulate webhook for refund.processed (partial refund: 40 of 100)
	payloadPartial := []byte(`{
		"id": "evt_ref_proc_1",
		"entity": "event",
		"event": "refund.processed",
		"payload": {
			"refund": {
				"entity": {
					"id": "rfnd_rzp_123",
					"payment_id": "pay_rzp_orig_100",
					"amount": 4000,
					"currency": "INR",
					"status": "processed",
					"notes": {
						"refund_id": "` + ref.ID + `",
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), payloadPartial); err != nil {
		t.Fatalf("webhook refund.processed failed: %v", err)
	}

	pUpdated, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if pUpdated.Status != model.PaymentStatusPartiallyRefunded {
		t.Errorf("expected payment status PARTIALLY_REFUNDED, got: %s", pUpdated.Status)
	}

	rUpdated, _ := repo.GetRefundByID(context.Background(), ref.ID)
	if rUpdated.Status != model.RefundStatusSuccess {
		t.Errorf("expected refund status SUCCESS, got: %s", rUpdated.Status)
	}

	// Second refund completing remaining 60.00
	ref2 := &model.Refund{
		PaymentID:       pmt.ID,
		OrderID:         pmt.OrderID,
		Amount:          60.00,
		Status:          model.RefundStatusPending,
		GatewayRefundID: "rfnd_rzp_456",
	}
	_ = repo.CreateRefund(context.Background(), ref2)

	payloadFull := []byte(`{
		"id": "evt_ref_proc_2",
		"entity": "event",
		"event": "refund.processed",
		"payload": {
			"refund": {
				"entity": {
					"id": "rfnd_rzp_456",
					"payment_id": "pay_rzp_orig_100",
					"amount": 6000,
					"currency": "INR",
					"status": "processed",
					"notes": {
						"refund_id": "` + ref2.ID + `",
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), payloadFull); err != nil {
		t.Fatalf("second webhook refund.processed failed: %v", err)
	}

	pFull, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if pFull.Status != model.PaymentStatusRefunded {
		t.Errorf("expected payment status REFUNDED after cumulative refunds complete, got: %s", pFull.Status)
	}
}

func TestHandleRazorpayWebhook_RefundFailed_SafelyHandled(t *testing.T) {
	repo := newMockPaymentRepo()
	orderClient := newMockOrderClient()
	gw := gateway.NewMockGateway(2 * time.Second)
	svc := service.NewPaymentService(repo, gw, orderClient)

	pmt := &model.Payment{
		OrderID:              "ord_wh_fail_test",
		Amount:               100.00,
		Currency:             "INR",
		Status:               model.PaymentStatusRefundPending,
		GatewayTransactionID: "pay_rzp_fail_orig",
		IdempotencyKey:       "idemp_wh_ref_fail",
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	ref := &model.Refund{
		PaymentID:       pmt.ID,
		OrderID:         pmt.OrderID,
		Amount:          50.00,
		Status:          model.RefundStatusPending,
		GatewayRefundID: "rfnd_rzp_fail_999",
	}
	_ = repo.CreateRefund(context.Background(), ref)

	payloadFail := []byte(`{
		"id": "evt_ref_fail_1",
		"entity": "event",
		"event": "refund.failed",
		"payload": {
			"refund": {
				"entity": {
					"id": "rfnd_rzp_fail_999",
					"payment_id": "pay_rzp_fail_orig",
					"status": "failed",
					"notes": {
						"refund_id": "` + ref.ID + `",
						"payment_id": "` + pmt.ID + `"
					}
				}
			}
		}
	}`)

	if err := svc.HandleRazorpayWebhook(context.Background(), payloadFail); err != nil {
		t.Fatalf("webhook refund.failed failed: %v", err)
	}

	rUpdated, _ := repo.GetRefundByID(context.Background(), ref.ID)
	if rUpdated.Status != model.RefundStatusFailed {
		t.Errorf("expected refund status FAILED, got: %s", rUpdated.Status)
	}

	pUpdated, _ := repo.GetPaymentByID(context.Background(), pmt.ID)
	if pUpdated.Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status to revert back to SUCCESS, got: %s", pUpdated.Status)
	}
}
