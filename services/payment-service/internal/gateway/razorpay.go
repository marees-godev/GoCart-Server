package gateway

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
)

type RazorpayGateway struct {
	keyID      string
	keySecret  string
	baseURL    string
	httpClient *http.Client
}

func NewRazorpayGateway(keyID, keySecret string, timeout time.Duration, baseURL ...string) *RazorpayGateway {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	url := "https://api.razorpay.com/v1"
	if len(baseURL) > 0 && baseURL[0] != "" {
		url = strings.TrimRight(baseURL[0], "/")
	}

	return &RazorpayGateway{
		keyID:     keyID,
		keySecret: keySecret,
		baseURL:   url,
		httpClient: &http.Client{
			Timeout: timeout,
		},
	}
}

func (r *RazorpayGateway) Provider() string {
	return "RAZORPAY"
}

// VerifyRazorpayWebhookSignature computes HMAC-SHA256 signature of body using secret and performs constant-time comparison.
func VerifyRazorpayWebhookSignature(body []byte, signature, secret string) bool {
	if secret == "" || signature == "" || len(body) == 0 {
		return false
	}
	h := hmac.New(sha256.New, []byte(secret))
	h.Write(body)
	expectedSignature := hex.EncodeToString(h.Sum(nil))
	return hmac.Equal([]byte(expectedSignature), []byte(signature))
}

func (r *RazorpayGateway) VerifyWebhookSignature(body []byte, signature string, secret ...string) bool {
	sec := r.keySecret
	if len(secret) > 0 && secret[0] != "" {
		sec = secret[0]
	}
	return VerifyRazorpayWebhookSignature(body, signature, sec)
}

func (r *RazorpayGateway) ProcessPayment(ctx context.Context, req *ProcessGatewayRequest) (*ProcessGatewayResponse, error) {
	if req == nil || req.OrderID == "" || req.Amount <= 0 {
		return nil, ErrInvalidGatewayInput
	}

	currency := strings.ToUpper(req.Currency)
	if currency == "" {
		currency = "INR"
	}

	// Convert float amount to smallest currency subunit (e.g. 250.75 -> 25075)
	amountInSubunits := int64(math.Round(req.Amount * 100))

	orderReqBody := model.RazorpayOrderRequest{
		Amount:   amountInSubunits,
		Currency: currency,
		Receipt:  req.OrderID,
		Notes: map[string]string{
			"payment_id":      req.PaymentID,
			"idempotency_key": req.IdempotencyKey,
		},
	}

	jsonBytes, err := json.Marshal(orderReqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal razorpay order payload: %w", err)
	}

	apiURL := fmt.Sprintf("%s/orders", r.baseURL)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create razorpay http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("X-Razorpay-Idempotency-Key", req.IdempotencyKey)
	}
	if r.keyID != "" && r.keySecret != "" {
		httpReq.SetBasicAuth(r.keyID, r.keySecret)
	}

	slog.InfoContext(ctx, "executing razorpay process payment request", "order_id", req.OrderID, "amount_subunits", amountInSubunits, "currency", currency)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		slog.WarnContext(ctx, "razorpay gateway http request failed", "order_id", req.OrderID, "error", err)
		return nil, SanitizeGatewayError(err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, SanitizeGatewayError(fmt.Errorf("failed to read razorpay response body: %w", err))
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp model.RazorpayOrderResponse
		_ = json.Unmarshal(bodyBytes, &errResp)
		failReason := "razorpay order creation failed"
		if errResp.Error != nil && errResp.Error.Description != "" {
			failReason = errResp.Error.Description
		}
		slog.WarnContext(ctx, "razorpay API returned non-success HTTP status", "status_code", resp.StatusCode, "reason", failReason)
		return &ProcessGatewayResponse{
			GatewayTransactionID: "",
			Status:               "FAILED",
			FailureReason:        failReason,
		}, nil
	}

	var razorpayResp model.RazorpayOrderResponse
	if err := json.Unmarshal(bodyBytes, &razorpayResp); err != nil {
		return nil, SanitizeGatewayError(fmt.Errorf("failed to parse razorpay order response: %w", err))
	}

	slog.InfoContext(ctx, "razorpay order created successfully", "razorpay_order_id", razorpayResp.ID, "status", razorpayResp.Status)
	return &ProcessGatewayResponse{
		GatewayTransactionID: razorpayResp.ID,
		Status:               "SUCCESS",
		FailureReason:        "",
	}, nil
}

func (r *RazorpayGateway) RefundPayment(ctx context.Context, req *RefundGatewayRequest) (*RefundGatewayResponse, error) {
	if req == nil || req.PaymentID == "" || req.Amount <= 0 {
		return nil, ErrInvalidGatewayInput
	}

	amountInSubunits := int64(math.Round(req.Amount * 100))

	refundReqBody := model.RazorpayRefundRequest{
		Amount: fmt.Sprintf("%d", amountInSubunits),
		Notes: map[string]string{
			"reason":          req.Reason,
			"idempotency_key": req.IdempotencyKey,
		},
	}

	jsonBytes, err := json.Marshal(refundReqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal razorpay refund payload: %w", err)
	}

	targetGatewayTxID := req.GatewayTransactionID
	if targetGatewayTxID == "" {
		targetGatewayTxID = req.PaymentID
	}

	apiURL := fmt.Sprintf("%s/payments/%s/refund", r.baseURL, targetGatewayTxID)
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create razorpay refund request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	if req.IdempotencyKey != "" {
		httpReq.Header.Set("X-Razorpay-Idempotency-Key", req.IdempotencyKey)
	}
	if r.keyID != "" && r.keySecret != "" {
		httpReq.SetBasicAuth(r.keyID, r.keySecret)
	}

	slog.InfoContext(ctx, "executing razorpay refund request", "payment_id", req.PaymentID, "gateway_tx_id", targetGatewayTxID, "amount_subunits", amountInSubunits)

	resp, err := r.httpClient.Do(httpReq)
	if err != nil {
		slog.WarnContext(ctx, "razorpay gateway refund http request failed", "payment_id", req.PaymentID, "error", err)
		return nil, SanitizeGatewayError(err)
	}
	defer resp.Body.Close()

	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, SanitizeGatewayError(fmt.Errorf("failed to read razorpay refund response body: %w", err))
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		var errResp model.RazorpayRefundResponse
		_ = json.Unmarshal(bodyBytes, &errResp)
		failReason := "razorpay refund failed"
		if errResp.Error != nil && errResp.Error.Description != "" {
			failReason = errResp.Error.Description
		}
		slog.WarnContext(ctx, "razorpay refund API returned non-success HTTP status", "status_code", resp.StatusCode, "reason", failReason)
		return &RefundGatewayResponse{
			GatewayRefundID: "",
			Status:          "FAILED",
			FailureReason:   failReason,
		}, nil
	}

	var refundResp model.RazorpayRefundResponse
	if err := json.Unmarshal(bodyBytes, &refundResp); err != nil {
		return nil, SanitizeGatewayError(fmt.Errorf("failed to parse razorpay refund response: %w", err))
	}

	slog.InfoContext(ctx, "razorpay refund processed successfully", "razorpay_refund_id", refundResp.ID, "status", refundResp.Status)
	return &RefundGatewayResponse{
		GatewayRefundID: refundResp.ID,
		Status:          "SUCCESS",
		FailureReason:   "",
	}, nil
}
