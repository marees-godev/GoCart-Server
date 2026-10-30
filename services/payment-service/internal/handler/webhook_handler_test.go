package handler_test

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gofiber/fiber/v2"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type mockRepo struct {
	payments        map[string]*model.Payment
	processedEvents map[string]bool
	updateCount     int
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		payments:        make(map[string]*model.Payment),
		processedEvents: make(map[string]bool),
	}
}

func (m *mockRepo) CreatePayment(ctx context.Context, p *model.Payment) error {
	p.ID = "pay_test_uuid_100"
	m.payments[p.ID] = p
	return nil
}

func (m *mockRepo) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	if p, ok := m.payments[id]; ok {
		return p, nil
	}
	return nil, appErrors.NotFound("payment not found")
}

func (m *mockRepo) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	for _, p := range m.payments {
		if p.OrderID == orderID {
			return p, nil
		}
	}
	return nil, appErrors.NotFound("payment for order not found")
}

func (m *mockRepo) GetPaymentByGatewayTransactionID(ctx context.Context, gatewayTxID string) (*model.Payment, error) {
	for _, p := range m.payments {
		if p.GatewayTransactionID == gatewayTxID {
			return p, nil
		}
	}
	return nil, appErrors.NotFound("payment for gateway transaction not found")
}

func (m *mockRepo) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	return nil, appErrors.NotFound("payment not found")
}

func (m *mockRepo) UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string, paymentMethod ...string) (*model.Payment, error) {
	if p, ok := m.payments[id]; ok {
		m.updateCount++
		p.Status = status
		p.GatewayTransactionID = gatewayTxID
		p.FailureReason = failureReason
		if len(paymentMethod) > 0 && paymentMethod[0] != "" {
			p.PaymentMethod = model.NormalizePaymentMethod(paymentMethod[0])
		}
		return p, nil
	}
	return nil, appErrors.NotFound("payment not found")
}

func (m *mockRepo) CreateRefund(ctx context.Context, ref *model.Refund) error { return nil }
func (m *mockRepo) GetRefundByID(ctx context.Context, id string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}
func (m *mockRepo) GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}
func (m *mockRepo) UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}
func (m *mockRepo) GetRefundByIdempotencyKey(ctx context.Context, key string) (*model.Refund, error) {
	return nil, appErrors.NotFound("refund not found")
}
func (m *mockRepo) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
	return nil, nil
}
func (m *mockRepo) GetTotalRefundedAmount(ctx context.Context, paymentID string) (float64, error) {
	return 0, nil
}
func (m *mockRepo) SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error { return nil }

func (m *mockRepo) IsWebhookProcessed(ctx context.Context, eventID string) (bool, error) {
	return m.processedEvents[eventID], nil
}

func (m *mockRepo) MarkWebhookProcessed(ctx context.Context, eventID string) error {
	m.processedEvents[eventID] = true
	return nil
}

type mockOrderClient struct{}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID string) (*client.AuthoritativeOrder, error) {
	return &client.AuthoritativeOrder{ID: orderID, TotalAmount: 100}, nil
}

func generateSignature(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func TestWebhookHandler_HandleRazorpayWebhook_InvalidSignature(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{"entity":"event","event":"payment.captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Razorpay-Signature", "invalid_signature")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for invalid signature, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_MissingSignature(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{"entity":"event","event":"payment.captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 for missing signature, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_EmptySecret(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	h := handler.NewWebhookHandler(svc, "")

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{"entity":"event","event":"payment.captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Razorpay-Signature", "any_signature")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 when webhook secret is empty, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_UnknownPayment(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_unknown_999",
					"notes": {
						"payment_id": "non_existent_payment_id"
					}
				}
			}
		}
	}`)

	sig := generateSignature(body, secret)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Razorpay-Signature", sig)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request: %v", err)
	}

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("expected status 404 for unknown payment, got %d", resp.StatusCode)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_Success(t *testing.T) {
	repo := newMockRepo()
	pmt := &model.Payment{
		ID:      "pay_test_uuid_100",
		OrderID: "ord_100",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_success_123",
					"notes": {
						"payment_id": "pay_test_uuid_100"
					}
				}
			}
		}
	}`)

	sig := generateSignature(body, secret)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Razorpay-Signature", sig)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("failed to test request: %v", err)
	}

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200 for valid webhook, got %d", resp.StatusCode)
	}

	if repo.payments["pay_test_uuid_100"].Status != model.PaymentStatusSuccess {
		t.Errorf("expected payment status SUCCESS, got %s", repo.payments["pay_test_uuid_100"].Status)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_DuplicateEvents(t *testing.T) {
	repo := newMockRepo()
	pmt := &model.Payment{
		ID:      "pay_test_uuid_100",
		OrderID: "ord_100",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{
		"id": "evt_duplicate_001",
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_success_123",
					"notes": {
						"payment_id": "pay_test_uuid_100"
					}
				}
			}
		}
	}`)

	sig := generateSignature(body, secret)

	// First event invocation
	req1 := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	req1.Header.Set("X-Razorpay-Signature", sig)

	resp1, err := app.Test(req1)
	if err != nil || resp1.StatusCode != http.StatusOK {
		t.Fatalf("first webhook failed: err=%v, code=%d", err, resp1.StatusCode)
	}

	if repo.updateCount != 1 {
		t.Fatalf("expected updateCount 1 after first webhook, got %d", repo.updateCount)
	}

	// Second duplicate event invocation
	req2 := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	req2.Header.Set("X-Razorpay-Signature", sig)

	resp2, err := app.Test(req2)
	if err != nil || resp2.StatusCode != http.StatusOK {
		t.Fatalf("duplicate webhook failed: err=%v, code=%d", err, resp2.StatusCode)
	}

	// Ensure no duplicate status transition occurred
	if repo.updateCount != 1 {
		t.Errorf("expected updateCount to remain 1 after duplicate webhook, got %d", repo.updateCount)
	}
}

func TestWebhookHandler_HandleRazorpayWebhook_Endpoint(t *testing.T) {
	repo := newMockRepo()
	pmt := &model.Payment{
		ID:      "pay_test_uuid_100",
		OrderID: "ord_100",
		Status:  model.PaymentStatusPending,
	}
	_ = repo.CreatePayment(context.Background(), pmt)

	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{
		"id": "evt_webhook_test",
		"entity": "event",
		"event": "payment.captured",
		"payload": {
			"payment": {
				"entity": {
					"id": "pay_rzp_webhook_test",
					"notes": {
						"payment_id": "pay_test_uuid_100"
					}
				}
			}
		}
	}`)
	sig := generateSignature(body, secret)
	req := httptest.NewRequest(http.MethodPost, "/webhooks", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Razorpay-Signature", sig)

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("request failed with err: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}
}
