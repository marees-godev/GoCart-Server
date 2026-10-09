package grpc_test

import (
	"context"
	"testing"
	"time"

	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	paymentGRPC "github.com/marees-godev/GoCart-Server/services/payment-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type mockPaymentService struct {
	payments map[string]*model.Payment
	refunds  map[string]*model.Refund
}

func newMockPaymentService() *mockPaymentService {
	return &mockPaymentService{
		payments: make(map[string]*model.Payment),
		refunds:  make(map[string]*model.Refund),
	}
}

func (m *mockPaymentService) ProcessPayment(ctx context.Context, dto *service.ProcessPaymentDTO) (*model.Payment, error) {
	if dto == nil || dto.OrderID == "" {
		return nil, appErrors.BadRequest("order_id is required")
	}

	p := &model.Payment{
		ID:                   "pay_grpc_123",
		OrderID:              dto.OrderID,
		UserID:               dto.UserID,
		PaymentMethod:        dto.PaymentMethod,
		Amount:               150.00, // Authoritative amount simulated
		Currency:             "USD",
		Status:               model.PaymentStatusSuccess,
		TransactionID:        "tx_grpc_123",
		GatewayTransactionID: "gtx_mock_grpc_123",
		IdempotencyKey:       dto.IdempotencyKey,
		CreatedAt:            time.Now(),
		UpdatedAt:            time.Now(),
	}

	m.payments[p.ID] = p
	m.payments["order_"+dto.OrderID] = p
	return p, nil
}

func (m *mockPaymentService) GetPaymentByID(ctx context.Context, id string) (*model.Payment, error) {
	p, ok := m.payments[id]
	if !ok {
		return nil, appErrors.NotFound("payment not found")
	}
	return p, nil
}

func (m *mockPaymentService) GetPaymentByOrderID(ctx context.Context, orderID string) (*model.Payment, error) {
	p, ok := m.payments["order_"+orderID]
	if !ok {
		return nil, appErrors.NotFound("payment not found for order")
	}
	return p, nil
}

func (m *mockPaymentService) CreateRefund(ctx context.Context, dto *service.CreateRefundDTO) (*model.Refund, error) {
	if dto == nil || dto.PaymentID == "" {
		return nil, appErrors.BadRequest("payment_id is required")
	}

	r := &model.Refund{
		ID:              "ref_grpc_123",
		PaymentID:       dto.PaymentID,
		OrderID:         "ord_123",
		Amount:          dto.Amount,
		Reason:          dto.Reason,
		Status:          model.RefundStatusSuccess,
		GatewayRefundID: "ref_mock_grpc_123",
		IdempotencyKey:  dto.IdempotencyKey,
		CreatedAt:       time.Now(),
		UpdatedAt:       time.Now(),
	}

	m.refunds[r.ID] = r
	return r, nil
}

func (m *mockPaymentService) HandleRazorpayWebhook(ctx context.Context, body []byte) error {
	return nil
}

func TestPaymentGRPCServer_ProcessPayment(t *testing.T) {
	mockSvc := newMockPaymentService()
	server := paymentGRPC.NewPaymentGRPCServer(mockSvc)

	req := &paymentpb.ProcessPaymentRequest{
		OrderId:        "ord_grpc_001",
		UserId:         "usr_grpc_001",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_grpc_001",
		Amount:         9999.00, // Should be overridden by service
	}

	resp, err := server.ProcessPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no gRPC error, got %v", err)
	}

	if resp.Payment == nil {
		t.Fatalf("expected payment in response, got nil")
	}

	if resp.Payment.OrderId != "ord_grpc_001" {
		t.Errorf("expected order_id ord_grpc_001, got %s", resp.Payment.OrderId)
	}

	if resp.Payment.Amount != 150.00 {
		t.Errorf("expected authoritative amount 150.00, got %.2f", resp.Payment.Amount)
	}

	if resp.Payment.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Payment.Status)
	}
}

func TestPaymentGRPCServer_GetPayment(t *testing.T) {
	mockSvc := newMockPaymentService()
	server := paymentGRPC.NewPaymentGRPCServer(mockSvc)

	// Process first
	_, _ = server.ProcessPayment(context.Background(), &paymentpb.ProcessPaymentRequest{
		OrderId:        "ord_grpc_002",
		UserId:         "usr_grpc_002",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_grpc_002",
	})

	// Get by ID
	resp, err := server.GetPayment(context.Background(), &paymentpb.GetPaymentRequest{
		Id: "pay_grpc_123",
	})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if resp.Payment.Id != "pay_grpc_123" {
		t.Errorf("expected payment id pay_grpc_123, got %s", resp.Payment.Id)
	}

	// Get by non-existent ID
	_, err = server.GetPayment(context.Background(), &paymentpb.GetPaymentRequest{
		Id: "non_existent",
	})
	if err == nil {
		t.Fatalf("expected error for non-existent payment id, got nil")
	}
}

func TestPaymentGRPCServer_CreateRefund(t *testing.T) {
	mockSvc := newMockPaymentService()
	server := paymentGRPC.NewPaymentGRPCServer(mockSvc)

	req := &paymentpb.CreateRefundRequest{
		PaymentId:      "pay_grpc_123",
		Amount:         50.00,
		Reason:         "defective item",
		IdempotencyKey: "ref_idemp_grpc_001",
	}

	resp, err := server.CreateRefund(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected refund status SUCCESS, got %s", resp.Status)
	}

	if resp.GatewayRefundId != "ref_mock_grpc_123" {
		t.Errorf("expected gateway refund id ref_mock_grpc_123, got %s", resp.GatewayRefundId)
	}
}
