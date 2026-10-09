package handler

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/gateway"
)

func TestPaymentHandler_GetKey(t *testing.T) {
	app := fiber.New()
	gwConfig := gateway.GatewayConfig{
		RazorpayKeyID: "rzp_test_12345",
	}
	h := NewPaymentHandler(nil, nil, gwConfig, slog.Default())
	h.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/get-key", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	var resultMap map[string]string
	_ = json.Unmarshal(body, &resultMap)

	if resultMap["key"] != "rzp_test_12345" {
		t.Errorf("expected rzp_test_12345, got %s", resultMap["key"])
	}
}

func TestPaymentHandler_PaymentCallback(t *testing.T) {
	app := fiber.New()
	h := NewPaymentHandler(nil, nil, gateway.GatewayConfig{}, slog.Default())
	h.RegisterRoutes(app)

	req := httptest.NewRequest("GET", "/payment-callback?razorpay_order_id=ord_123&razorpay_payment_id=pay_456&razorpay_signature=sig_789", nil)
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	var resultMap map[string]string
	_ = json.Unmarshal(body, &resultMap)

	if resultMap["status"] != "success" || resultMap["order_id"] != "ord_123" || resultMap["payment_id"] != "pay_456" {
		t.Errorf("unexpected callback response: %v", resultMap)
	}
}

func TestPaymentHandler_CreateOrder_TestMode(t *testing.T) {
	app := fiber.New()
	h := NewPaymentHandler(nil, nil, gateway.GatewayConfig{}, slog.Default())
	h.RegisterRoutes(app)

	req := httptest.NewRequest("POST", "/create-order", strings.NewReader(`{"amount": 1000, "currency": "INR"}`))
	req.Header.Set("Content-Type", "application/json")
	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	body, _ := io.ReadAll(resp.Body)
	var resultMap map[string]interface{}
	_ = json.Unmarshal(body, &resultMap)

	if resultMap["status"] != "created" || resultMap["is_real"] != false {
		t.Errorf("unexpected create order response: %v", resultMap)
	}
}
