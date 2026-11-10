package provider

import (
	"context"
	"strings"
	"sync"
	"time"
)

type InAppProvider struct {
	mu   sync.RWMutex
	sent []Notification
}

func NewInAppProvider() *InAppProvider {
	return &InAppProvider{
		sent: make([]Notification, 0),
	}
}

func (p *InAppProvider) Channel() string {
	return "IN_APP"
}

func (p *InAppProvider) ProviderName() string {
	return "in-app"
}

func (p *InAppProvider) SentNotifications() []Notification {
	p.mu.RLock()
	defer p.mu.RUnlock()
	copied := make([]Notification, len(p.sent))
	copy(copied, p.sent)
	return copied
}

func (p *InAppProvider) Clear() {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.sent = make([]Notification, 0)
}

func (p *InAppProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
		return ErrInvalidRecipient
	}

	select {
	case <-ctx.Done():
		return ErrProviderTimeout
	default:
	}

	p.mu.Lock()
	p.sent = append(p.sent, notification)
	p.mu.Unlock()

	return nil
}

