package gateway

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
)

type MockGateway struct {
	timeout time.Duration
}

func NewMockGateway(timeout time.Duration) *MockGateway {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	return &MockGateway{
		timeout: timeout,
	}
}

func (m *MockGateway) Provider() string {
	return "MOCK"
}

func (m *MockGateway) ProcessPayment(ctx context.Context, req *ProcessGatewayRequest) (*ProcessGatewayResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	slog.InfoContext(ctx, "MockGateway ProcessPayment initiated", "payment_id", req.PaymentID, "order_id", req.OrderID, "amount", req.Amount)

	select {
	case <-ctx.Done():
		slog.WarnContext(ctx, "MockGateway ProcessPayment timed out", "payment_id", req.PaymentID)
		return nil, SanitizeGatewayError(ctx.Err())
	default:
	}

	if req == nil || req.OrderID == "" || req.Amount <= 0 {
		slog.WarnContext(ctx, "MockGateway ProcessPayment received invalid input", "payment_id", req.PaymentID)
		return nil, ErrInvalidGatewayInput
	}

	// Trigger simulation via idempotency key for testing
	if strings.Contains(req.IdempotencyKey, "fail_gateway") {
		gtx := fmt.Sprintf("gtx_mock_failed_%s", uuid.New().String()[:8])
		slog.InfoContext(ctx, "MockGateway simulated payment decline", "payment_id", req.PaymentID, "gateway_tx_id", gtx)
		return &ProcessGatewayResponse{
			GatewayTransactionID: gtx,
			Status:               "FAILED",
			FailureReason:        "Insufficient funds in mock card",
		}, nil
	}

	if strings.Contains(req.IdempotencyKey, "timeout_gateway") {
		slog.InfoContext(ctx, "MockGateway simulating timeout", "payment_id", req.PaymentID)
		time.Sleep(m.timeout + 10*time.Millisecond)
		return nil, ErrGatewayTimeout
	}

	txID := fmt.Sprintf("gtx_mock_%s", uuid.New().String())
	slog.InfoContext(ctx, "MockGateway ProcessPayment succeeded", "payment_id", req.PaymentID, "gateway_tx_id", txID)
	return &ProcessGatewayResponse{
		GatewayTransactionID: txID,
		Status:               "SUCCESS",
		FailureReason:        "",
	}, nil
}

func (m *MockGateway) RefundPayment(ctx context.Context, req *RefundGatewayRequest) (*RefundGatewayResponse, error) {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	slog.InfoContext(ctx, "MockGateway RefundPayment initiated", "refund_id", req.RefundID, "payment_id", req.PaymentID, "amount", req.Amount)

	select {
	case <-ctx.Done():
		slog.WarnContext(ctx, "MockGateway RefundPayment timed out", "refund_id", req.RefundID)
		return nil, SanitizeGatewayError(ctx.Err())
	default:
	}

	if req == nil || req.PaymentID == "" || req.Amount <= 0 {
		slog.WarnContext(ctx, "MockGateway RefundPayment received invalid input", "refund_id", req.RefundID)
		return nil, ErrInvalidGatewayInput
	}

	refundID := fmt.Sprintf("ref_mock_%s", uuid.New().String())
	slog.InfoContext(ctx, "MockGateway RefundPayment succeeded", "refund_id", req.RefundID, "gateway_refund_id", refundID)
	return &RefundGatewayResponse{
		GatewayRefundID: refundID,
		Status:          "SUCCESS",
		FailureReason:   "",
	}, nil
}
