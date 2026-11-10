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

type PushConfig struct {
	Provider  string
	ServerKey string
	Timeout   time.Duration
}

type MockPushProvider struct {
	mu       sync.RWMutex
	timeout  time.Duration
	sent     []Notification
	failNext error
}

func NewMockPushProvider(timeout time.Duration) *MockPushProvider {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &MockPushProvider{
		timeout: timeout,
		sent:    make([]Notification, 0),
	}
}

func (m *MockPushProvider) Channel() string {
	return "PUSH"
}

func (m *MockPushProvider) ProviderName() string {
	return "mock-push"
}

func (m *MockPushProvider) SetFailNext(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = err
}

func (m *MockPushProvider) SentNotifications() []Notification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make([]Notification, len(m.sent))
	copy(copied, m.sent)
	return copied
}

func (m *MockPushProvider) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = make([]Notification, 0)
	m.failNext = nil
}

func (m *MockPushProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
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

type FCMPushProvider struct {
	config     PushConfig
	httpClient *http.Client
}

func NewFCMPushProvider(cfg PushConfig) *FCMPushProvider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 5 * time.Second
	}
	return &FCMPushProvider{
		config: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
	}
}

func (p *FCMPushProvider) Channel() string {
	return "PUSH"
}

func (p *FCMPushProvider) ProviderName() string {
	return "fcm"
}

func (p *FCMPushProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
		return ErrInvalidRecipient
	}

	if p.config.ServerKey == "" {
		return fmt.Errorf("%w: missing FCM server key", ErrProviderMisconfigured)
	}

	select {
	case <-ctx.Done():
		return ErrProviderTimeout
	default:
	}

	// Simulated / stubbed HTTP API call
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, "https://fcm.googleapis.com/fcm/send", nil)
	if err != nil {
		return p.sanitizeError(err)
	}
	_ = req

	return nil
}

func (p *FCMPushProvider) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	raw := err.Error()
	if p.config.ServerKey != "" {
		raw = strings.ReplaceAll(raw, p.config.ServerKey, "******")
	}
	sanitized := SanitizeError(err, ErrDeliveryFailed)
	if errors.Is(sanitized, ErrProviderTimeout) || errors.Is(sanitized, ErrProviderUnavailable) || errors.Is(sanitized, ErrInvalidRecipient) {
		return sanitized
	}
	return fmt.Errorf("%w: %s", ErrDeliveryFailed, raw)
}

