package service_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

// In-Memory Repository Mock
type mockPaymentRepo struct {
	mu       sync.Mutex
	payments map[string]*model.Payment
	idempMap map[string]*model.Payment
	orderMap map[string]*model.Payment
	refunds  map[string]*model.Refund
	nextID   int
}

func newMockPaymentRepo() *mockPaymentRepo {
	return &mockPaymentRepo{
		payments: make(map[string]*model.Payment),
		idempMap: make(map[string]*model.Payment),
		orderMap: make(map[string]*model.Payment),
		refunds:  make(map[string]*model.Refund),
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
	return false, nil
}

func (m *mockPaymentRepo) MarkWebhookProcessed(ctx context.Context, eventID string) error {
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
	_, _ = repo.UpdatePaymentStatus(context.Background(), payment.ID, model.PaymentStatusPending, "", "")

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
	_, _ = repo.UpdatePaymentStatus(context.Background(), payment.ID, model.PaymentStatusPending, "", "")

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
