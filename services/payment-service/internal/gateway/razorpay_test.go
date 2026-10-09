package gateway_test

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
)

func TestRazorpayGateway_ProcessPayment_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/orders" {
			t.Errorf("unexpected request path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		user, pass, ok := r.BasicAuth()
		if !ok || user != "rzp_test_key" || pass != "rzp_test_secret" {
			t.Errorf("invalid basic auth header")
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		if r.Header.Get("X-Razorpay-Idempotency-Key") != "idemp_rzp_001" {
			t.Errorf("expected idempotency key header idemp_rzp_001, got %s", r.Header.Get("X-Razorpay-Idempotency-Key"))
		}

		w.WriteHeader(http.StatusCreated)
		_, _ = w.Write([]byte(`{
			"id": "order_RZP123456789",
			"entity": "order",
			"amount": 25075,
			"currency": "INR",
			"receipt": "ord_1001",
			"status": "created"
		}`))
	}))
	defer ts.Close()

	gw := gateway.NewRazorpayGateway("rzp_test_key", "rzp_test_secret", 2*time.Second, ts.URL)

	if gw.Provider() != "RAZORPAY" {
		t.Errorf("expected provider RAZORPAY, got %s", gw.Provider())
	}

	req := &gateway.ProcessGatewayRequest{
		PaymentID:      "pay_1001",
		OrderID:        "ord_1001",
		Amount:         250.75,
		Currency:       "INR",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_rzp_001",
	}

	resp, err := gw.ProcessPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}

	if resp.GatewayTransactionID != "order_RZP123456789" {
		t.Errorf("expected gateway transaction ID order_RZP123456789, got %s", resp.GatewayTransactionID)
	}
}

func TestRazorpayGateway_ProcessPayment_Decline(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{
			"error": {
				"code": "BAD_REQUEST_ERROR",
				"description": "Order amount exceeds limit"
			}
		}`))
	}))
	defer ts.Close()

	gw := gateway.NewRazorpayGateway("rzp_test_key", "rzp_test_secret", 2*time.Second, ts.URL)

	req := &gateway.ProcessGatewayRequest{
		PaymentID:      "pay_fail_rzp",
		OrderID:        "ord_fail_rzp",
		Amount:         9999999.00,
		Currency:       "INR",
		PaymentMethod:  "CREDIT_CARD",
		IdempotencyKey: "idemp_fail_rzp",
	}

	resp, err := gw.ProcessPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected response object, got err: %v", err)
	}

	if resp.Status != "FAILED" {
		t.Errorf("expected status FAILED, got %s", resp.Status)
	}

	if resp.FailureReason != "Order amount exceeds limit" {
		t.Errorf("expected failure reason 'Order amount exceeds limit', got '%s'", resp.FailureReason)
	}
}

func TestRazorpayGateway_RefundPayment_Success(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/payments/order_RZP123456789/refund" {
			t.Errorf("unexpected refund path: %s", r.URL.Path)
			http.Error(w, "not found", http.StatusNotFound)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{
			"id": "rfnd_RZP987654321",
			"entity": "refund",
			"amount": 5000,
			"currency": "INR",
			"payment_id": "order_RZP123456789",
			"status": "processed"
		}`))
	}))
	defer ts.Close()

	gw := gateway.NewRazorpayGateway("rzp_test_key", "rzp_test_secret", 2*time.Second, ts.URL)

	req := &gateway.RefundGatewayRequest{
		RefundID:             "ref_1001",
		PaymentID:            "pay_1001",
		GatewayTransactionID: "order_RZP123456789",
		Amount:               50.00,
		Reason:               "Item damaged",
		IdempotencyKey:       "ref_idemp_001",
	}

	resp, err := gw.RefundPayment(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Status != "SUCCESS" {
		t.Errorf("expected status SUCCESS, got %s", resp.Status)
	}

	if resp.GatewayRefundID != "rfnd_RZP987654321" {
		t.Errorf("expected gateway refund ID rfnd_RZP987654321, got %s", resp.GatewayRefundID)
	}
}

func TestVerifyRazorpayWebhookSignature(t *testing.T) {
	secret := "whsec_test_secret_999"
	body := []byte(`{"entity":"event","event":"payment.captured"}`)

	// Compute valid signature
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	validSig := hex.EncodeToString(h.Sum(nil))

	if !gateway.VerifyRazorpayWebhookSignature(body, validSig, secret) {
		t.Errorf("expected VerifyRazorpayWebhookSignature to return true for valid signature")
	}

	invalidSig := "invalid_signature_hex_string"
	if gateway.VerifyRazorpayWebhookSignature(body, invalidSig, secret) {
		t.Errorf("expected VerifyRazorpayWebhookSignature to return false for invalid signature")
	}

	if gateway.VerifyRazorpayWebhookSignature(body, validSig, "wrong_secret") {
		t.Errorf("expected VerifyRazorpayWebhookSignature to return false for wrong secret")
	}

	if gateway.VerifyRazorpayWebhookSignature(nil, validSig, secret) {
		t.Errorf("expected VerifyRazorpayWebhookSignature to return false for nil body")
	}
}
