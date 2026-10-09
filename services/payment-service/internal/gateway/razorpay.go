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

// Webhook payload structures for Razorpay events
type RazorpayWebhookEvent struct {
	Entity    string                 `json:"entity"`
	AccountID string                 `json:"account_id"`
	Event     string                 `json:"event"`
	Contains  []string               `json:"contains"`
	Payload   RazorpayWebhookPayload `json:"payload"`
	CreatedAt int64                  `json:"created_at"`
}

type RazorpayWebhookPayload struct {
	Payment RazorpayPaymentContainer `json:"payment"`
	Order   RazorpayOrderContainer   `json:"order"`
	Refund  RazorpayRefundContainer  `json:"refund"`
}

type RazorpayPaymentContainer struct {
	Entity RazorpayPaymentEntity `json:"entity"`
}

type RazorpayOrderContainer struct {
	Entity RazorpayOrderEntity `json:"entity"`
}

type RazorpayRefundContainer struct {
	Entity RazorpayRefundEntity `json:"entity"`
}

type RazorpayRefundEntity struct {
	ID        string            `json:"id"`
	Entity    string            `json:"entity"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	PaymentID string            `json:"payment_id"`
	Notes     map[string]string `json:"notes"`
	Status    string            `json:"status"`
	CreatedAt int64             `json:"created_at"`
}

type RazorpayPaymentEntity struct {
	ID               string            `json:"id"`
	Entity           string            `json:"entity"`
	Amount           int64             `json:"amount"`
	Currency         string            `json:"currency"`
	Status           string            `json:"status"`
	OrderID          string            `json:"order_id"`
	InvoiceID        string            `json:"invoice_id"`
	Method           string            `json:"method"`
	Captured         bool              `json:"captured"`
	Description      string            `json:"description"`
	Notes            map[string]string `json:"notes"`
	ErrorCode        string            `json:"error_code"`
	ErrorDescription string            `json:"error_description"`
	ErrorSource      string            `json:"error_source"`
	ErrorStep        string            `json:"error_step"`
	ErrorReason      string            `json:"error_reason"`
	CreatedAt        int64             `json:"created_at"`
}

type RazorpayOrderEntity struct {
	ID        string            `json:"id"`
	Entity    string            `json:"entity"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Receipt   string            `json:"receipt"`
	Status    string            `json:"status"`
	Notes     map[string]string `json:"notes"`
	CreatedAt int64             `json:"created_at"`
}

type razorpayOrderRequest struct {
	Amount   int64             `json:"amount"` // Amount in smallest currency unit (paise / cents)
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type razorpayOrderResponse struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Receipt   string `json:"receipt"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	Error     *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Step        string `json:"step"`
		Reason      string `json:"reason"`
	} `json:"error,omitempty"`
}

type razorpayRefundRequest struct {
	Amount string            `json:"amount,omitempty"`
	Notes  map[string]string `json:"notes,omitempty"`
}

type razorpayRefundResponse struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Error     *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
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

	orderReqBody := razorpayOrderRequest{
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
		var errResp razorpayOrderResponse
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

	var razorpayResp razorpayOrderResponse
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

	refundReqBody := razorpayRefundRequest{
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
		var errResp razorpayRefundResponse
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

	var refundResp razorpayRefundResponse
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
