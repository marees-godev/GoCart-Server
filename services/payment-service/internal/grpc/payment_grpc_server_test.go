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

func (m *mockPaymentService) CancelPayment(ctx context.Context, dto *service.CancelPaymentDTO) (*model.Payment, error) {
	if dto == nil || (dto.PaymentID == "" && dto.OrderID == "") {
		return nil, appErrors.BadRequest("payment_id or order_id is required")
	}
	pID := dto.PaymentID
	if pID == "" {
		pID = dto.OrderID
	}
	p, ok := m.payments[pID]
	if !ok {
		return nil, appErrors.NotFound("payment not found")
	}
	if p.Status != model.PaymentStatusInitiated && p.Status != model.PaymentStatusPending {
		return nil, appErrors.BadRequest("cannot cancel payment")
	}
	p.Status = model.PaymentStatusCancelled
	p.FailureReason = dto.Reason
	return p, nil
}

func (m *mockPaymentService) GetRefundsByPaymentID(ctx context.Context, paymentID string) ([]*model.Refund, error) {
	var res []*model.Refund
	for _, r := range m.refunds {
		if r.PaymentID == paymentID {
			res = append(res, r)
		}
	}
	return res, nil
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
	if resp.PaymentId != "pay_grpc_123" {
		t.Errorf("expected payment id pay_grpc_123, got %s", resp.PaymentId)
	}
}

func TestPaymentGRPCServer_CancelPayment(t *testing.T) {
	mockSvc := newMockPaymentService()
	server := paymentGRPC.NewPaymentGRPCServer(mockSvc)

	// Setup pending payment
	payID := "pay_to_cancel"
	mockSvc.payments[payID] = &model.Payment{
		ID:      payID,
		OrderID: "ord_cancel_1",
		Amount:  100.0,
		Status:  model.PaymentStatusPending,
	}

	req := &paymentpb.CancelPaymentRequest{
		PaymentId: payID,
		Reason:    "Customer changed mind",
	}

	resp, err := server.CancelPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error cancelling payment, got %v", err)
	}

	if resp.Payment.Status != string(model.PaymentStatusCancelled) {
		t.Errorf("expected payment status CANCELLED, got %s", resp.Payment.Status)
	}
	if resp.Payment.FailureReason != "Customer changed mind" {
		t.Errorf("expected failure reason 'Customer changed mind', got %s", resp.Payment.FailureReason)
	}
}

func TestPaymentGRPCServer_GetRefundsByPaymentID(t *testing.T) {
	mockSvc := newMockPaymentService()
	server := paymentGRPC.NewPaymentGRPCServer(mockSvc)

	payID := "pay_with_refunds"
	mockSvc.refunds["ref_1"] = &model.Refund{
		ID:        "ref_1",
		PaymentID: payID,
		OrderID:   "ord_1",
		Amount:    25.0,
		Status:    model.RefundStatusSuccess,
		CreatedAt: time.Now(),
		UpdatedAt: time.Now(),
	}

	req := &paymentpb.GetRefundsByPaymentIDRequest{
		PaymentId: payID,
	}

	resp, err := server.GetRefundsByPaymentID(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error getting refunds, got %v", err)
	}

	if len(resp.Refunds) != 1 {
		t.Fatalf("expected 1 refund, got %d", len(resp.Refunds))
	}
	if resp.Refunds[0].Id != "ref_1" || resp.Refunds[0].Amount != 25.0 {
		t.Errorf("unexpected refund data: %+v", resp.Refunds[0])
	}
}

