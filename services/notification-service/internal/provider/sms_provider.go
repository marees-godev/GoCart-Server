package provider

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"
)

type SMSConfig struct {
	Provider   string
	AccountSID string
	AuthToken  string
	FromNumber string
	Timeout    time.Duration
}

type MockSMSProvider struct {
	mu       sync.RWMutex
	timeout  time.Duration
	sent     []Notification
	failNext error
}

func NewMockSMSProvider(timeout time.Duration) *MockSMSProvider {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &MockSMSProvider{
		timeout: timeout,
		sent:    make([]Notification, 0),
	}
}

func (m *MockSMSProvider) Channel() string {
	return "SMS"
}

func (m *MockSMSProvider) ProviderName() string {
	return "mock-sms"
}

func (m *MockSMSProvider) SetFailNext(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = err
}

func (m *MockSMSProvider) SentNotifications() []Notification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make([]Notification, len(m.sent))
	copy(copied, m.sent)
	return copied
}

func (m *MockSMSProvider) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = make([]Notification, 0)
	m.failNext = nil
}

func (m *MockSMSProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" || len(recipient) < 5 {
		return ErrInvalidRecipient
	}

	m.mu.Lock()
	failErr := m.failNext
	m.failNext = nil
	m.mu.Unlock()

	if ctx.Err() != nil {
		return ErrProviderTimeout
	}

	if failErr != nil {
		return SanitizeError(failErr, ErrDeliveryFailed)
	}

	m.mu.Lock()
	m.sent = append(m.sent, notification)
	m.mu.Unlock()

	return nil
}

type TwilioSMSProvider struct {
	config     SMSConfig
	httpClient *http.Client
}

func NewTwilioSMSProvider(cfg SMSConfig) *TwilioSMSProvider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &TwilioSMSProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *TwilioSMSProvider) Channel() string {
	return "SMS"
}

func (p *TwilioSMSProvider) ProviderName() string {
	return "twilio"
}

func (p *TwilioSMSProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" || len(recipient) < 5 {
		return ErrInvalidRecipient
	}

	if p.config.AccountSID == "" || p.config.AuthToken == "" {
		return fmt.Errorf("%w: missing Twilio credentials", ErrProviderMisconfigured)
	}

	// Verify context deadline
	select {
	case <-ctx.Done():
		return ErrProviderTimeout
	default:
	}

	// Simulated / stubbed HTTP API call
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://api.twilio.com/2010-04-01/Accounts/"+p.config.AccountSID+"/Messages.json", nil)
	if err != nil {
		return p.sanitizeError(err)
	}
	_ = req

	return nil
}

func (p *TwilioSMSProvider) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	raw := err.Error()
	if p.config.AuthToken != "" {
		raw = strings.ReplaceAll(raw, p.config.AuthToken, "******")
	}
	if p.config.AccountSID != "" {
		raw = strings.ReplaceAll(raw, p.config.AccountSID, "******")
	}
	sanitized := SanitizeError(err, ErrDeliveryFailed)
	if errors.Is(sanitized, ErrProviderTimeout) || errors.Is(sanitized, ErrProviderUnavailable) || errors.Is(sanitized, ErrInvalidRecipient) {
		return sanitized
	}
	return fmt.Errorf("%w: %s", ErrDeliveryFailed, raw)
}

