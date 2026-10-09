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
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type mockRepo struct {
	payments map[string]*model.Payment
}

func newMockRepo() *mockRepo {
	return &mockRepo{payments: make(map[string]*model.Payment)}
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
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	for _, p := range m.payments {
		if p.OrderID == orderID {
			return p, nil
		}
	}
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) GetPaymentByGatewayTransactionID(ctx context.Context, gatewayTxID string) (*model.Payment, error) {
	for _, p := range m.payments {
		if p.GatewayTransactionID == gatewayTxID {
			return p, nil
		}
	}
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) GetPaymentByIdempotencyKey(ctx context.Context, key string) (*model.Payment, error) {
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) UpdatePaymentStatus(ctx context.Context, id string, status model.PaymentStatus, gatewayTxID string, failureReason string) (*model.Payment, error) {
	if p, ok := m.payments[id]; ok {
		p.Status = status
		p.GatewayTransactionID = gatewayTxID
		p.FailureReason = failureReason
		return p, nil
	}
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) CreateRefund(ctx context.Context, ref *model.Refund) error { return nil }
func (m *mockRepo) GetRefundByID(ctx context.Context, id string) (*model.Refund, error) {
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) GetRefundByGatewayRefundID(ctx context.Context, gatewayRefundID string) (*model.Refund, error) {
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) UpdateRefundStatus(ctx context.Context, id string, status model.RefundStatus, gatewayRefundID string) (*model.Refund, error) {
	return nil, fiber.ErrNotFound
}
func (m *mockRepo) SaveWebhookEvent(ctx context.Context, w *model.PaymentWebhook) error { return nil }
func (m *mockRepo) IsWebhookProcessed(ctx context.Context, eventID string) (bool, error) {
	return false, nil
}
func (m *mockRepo) MarkWebhookProcessed(ctx context.Context, eventID string) error { return nil }

type mockOrderClient struct{}

func (m *mockOrderClient) GetOrder(ctx context.Context, orderID string) (*client.AuthoritativeOrder, error) {
	return &client.AuthoritativeOrder{ID: orderID, TotalAmount: 100}, nil
}

func TestWebhookHandler_HandleRazorpayWebhook_InvalidSignature(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewPaymentService(repo, gateway.NewMockGateway(time.Second), &mockOrderClient{})
	secret := "secret_key_123"
	h := handler.NewWebhookHandler(svc, secret)

	app := fiber.New()
	h.RegisterRoutes(app)

	body := []byte(`{"entity":"event","event":"payment.captured"}`)
	req := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", bytes.NewReader(body))
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

	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	sig := hex.EncodeToString(mac.Sum(nil))

	req := httptest.NewRequest(http.MethodPost, "/webhooks/razorpay", bytes.NewReader(body))
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
