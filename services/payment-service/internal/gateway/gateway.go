package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrGatewayTimeout      = errors.New("payment gateway request timed out")
	ErrGatewayDeclined     = errors.New("payment declined by gateway")
	ErrGatewayUnavailable  = errors.New("payment gateway is currently unavailable")
	ErrInvalidGatewayInput = errors.New("invalid input provided to gateway")
)

type ProcessGatewayRequest struct {
	PaymentID      string
	OrderID        string
	Amount         float64
	Currency       string
	PaymentMethod  string
	IdempotencyKey string
}

type ProcessGatewayResponse struct {
	GatewayTransactionID string
	Status               string // SUCCESS, FAILED, PENDING
	FailureReason        string
}

type RefundGatewayRequest struct {
	RefundID             string
	PaymentID            string
	GatewayTransactionID string
	Amount               float64
	Reason               string
	IdempotencyKey       string
}

type RefundGatewayResponse struct {
	GatewayRefundID string
	Status          string // SUCCESS, FAILED
	FailureReason   string
}

// PaymentGateway abstracts external payment providers (e.g. Mock, Stripe, PayPal, Razorpay).
type PaymentGateway interface {
	ProcessPayment(ctx context.Context, req *ProcessGatewayRequest) (*ProcessGatewayResponse, error)
	RefundPayment(ctx context.Context, req *RefundGatewayRequest) (*RefundGatewayResponse, error)
	Provider() string
}

type GatewayConfig struct {
	Provider              string
	APIKey                string
	RazorpayKeyID         string
	RazorpayKeySecret     string
	RazorpayWebhookSecret string
	Timeout               time.Duration
}

func NewPaymentGateway(cfg GatewayConfig) PaymentGateway {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	switch strings.ToLower(cfg.Provider) {
	case "razorpay":
		keyID := cfg.RazorpayKeyID
		if keyID == "" {
			keyID = cfg.APIKey
		}
		return NewRazorpayGateway(keyID, cfg.RazorpayKeySecret, cfg.Timeout)
	case "mock", "":
		return NewMockGateway(cfg.Timeout)
	default:
		return NewMockGateway(cfg.Timeout)
	}
}

// SanitizeGatewayError wraps raw gateway errors to prevent leaking credentials or internal stack traces.
func SanitizeGatewayError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, ErrGatewayTimeout) {
		return ErrGatewayTimeout
	}
	if errors.Is(err, ErrGatewayDeclined) || errors.Is(err, ErrInvalidGatewayInput) {
		return err
	}
	// Default sanitized generic gateway error
	return fmt.Errorf("%w: processing failed", ErrGatewayUnavailable)
}
