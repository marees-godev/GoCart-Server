package provider

import (
	"context"
	"net/mail"
	"strings"
	"sync"
	"time"
)

type MockEmailProvider struct {
	mu       sync.RWMutex
	timeout  time.Duration
	sent     []Notification
	failNext error
	latency  time.Duration
}

func NewMockEmailProvider(timeout time.Duration) *MockEmailProvider {
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	return &MockEmailProvider{
		timeout: timeout,
		sent:    make([]Notification, 0),
	}
}

func (m *MockEmailProvider) Channel() string {
	return "EMAIL"
}

func (m *MockEmailProvider) ProviderName() string {
	return "mock-email"
}

func (m *MockEmailProvider) SetFailNext(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.failNext = err
}

func (m *MockEmailProvider) SetLatency(d time.Duration) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.latency = d
}

func (m *MockEmailProvider) Clear() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sent = make([]Notification, 0)
	m.failNext = nil
	m.latency = 0
}

func (m *MockEmailProvider) SentNotifications() []Notification {
	m.mu.RLock()
	defer m.mu.RUnlock()
	copied := make([]Notification, len(m.sent))
	copy(copied, m.sent)
	return copied
}

func (m *MockEmailProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, m.timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
		return ErrInvalidRecipient
	}
	if _, err := mail.ParseAddress(recipient); err != nil && !strings.Contains(recipient, "@") {
		return ErrInvalidRecipient
	}

	m.mu.Lock()
	latency := m.latency
	failErr := m.failNext
	m.failNext = nil
	m.mu.Unlock()

	if latency > 0 {
		select {
		case <-time.After(latency):
		case <-ctx.Done():
			return ErrProviderTimeout
		}
	}

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

