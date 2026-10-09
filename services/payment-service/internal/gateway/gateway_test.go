package gateway_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
)

func TestMockGateway_ProcessPayment_Success(t *testing.T) {
	gw := gateway.NewPaymentGateway(gateway.GatewayConfig{
		Provider: "mock",
		Timeout:  2 * time.Second,
	})

	req := &gateway.ProcessGatewayRequest{
		PaymentID:      "pay_123",
		OrderID:        "ord_123",
		Amount:         100.50,
		Currency:       "USD",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_test_success",
	}

	resp, err := gw.ProcessPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}
	if resp.GatewayTransactionID == "" {
		t.Errorf("expected non-empty gateway transaction id")
	}
}

func TestMockGateway_ProcessPayment_FailureSimulation(t *testing.T) {
	gw := gateway.NewPaymentGateway(gateway.GatewayConfig{
		Provider: "mock",
		Timeout:  2 * time.Second,
	})

	req := &gateway.ProcessGatewayRequest{
		PaymentID:      "pay_fail",
		OrderID:        "ord_fail",
		Amount:         50.00,
		Currency:       "USD",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_fail_gateway",
	}

	resp, err := gw.ProcessPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no go error, got %v", err)
	}

	if resp.Status != "FAILED" {
		t.Errorf("expected status FAILED, got %s", resp.Status)
	}
	if resp.FailureReason == "" {
		t.Errorf("expected failure reason for failed transaction")
	}
}

func TestMockGateway_ProcessPayment_TimeoutSimulation(t *testing.T) {
	gw := gateway.NewPaymentGateway(gateway.GatewayConfig{
		Provider: "mock",
		Timeout:  50 * time.Millisecond,
	})

	req := &gateway.ProcessGatewayRequest{
		PaymentID:      "pay_timeout",
		OrderID:        "ord_timeout",
		Amount:         75.00,
		Currency:       "USD",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_timeout_gateway",
	}

	_, err := gw.ProcessPayment(context.Background(), req)
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}

	if !errors.Is(err, gateway.ErrGatewayTimeout) {
		t.Errorf("expected ErrGatewayTimeout, got %v", err)
	}
}

func TestSanitizeGatewayError(t *testing.T) {
	rawErr := errors.New("secret_api_key=sk_live_12345 connection database failed")
	sanitized := gateway.SanitizeGatewayError(rawErr)

	if sanitized.Error() == rawErr.Error() {
		t.Errorf("expected raw error to be sanitized and not match raw string")
	}

	if !errors.Is(sanitized, gateway.ErrGatewayUnavailable) {
		t.Errorf("expected ErrGatewayUnavailable, got %v", sanitized)
	}
}
