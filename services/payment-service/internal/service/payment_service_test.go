package service_test

import (
	"context"
	"errors"
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
	p.ID = "pay_uuid_" + string(rune(m.nextID+'0'))
	if p.PaymentMethodID == nil || *p.PaymentMethodID == "" {
		pmID := "a0eebc99-9c0b-4ef8-bb6d-6bb9bd38000" + string(rune(m.nextID+'0'))
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
		if status == model.PaymentStatusSuccess {
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
		} else if status == model.PaymentStatusFailed {
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

	m.nextID++
	ref.ID = "ref_uuid_" + string(rune(m.nextID+'0'))
	ref.CreatedAt = time.Now()
	ref.UpdatedAt = time.Now()

	refCopy := *ref
	m.refunds[ref.ID] = &refCopy
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
	r.GatewayRefundID = gatewayRefundID
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
