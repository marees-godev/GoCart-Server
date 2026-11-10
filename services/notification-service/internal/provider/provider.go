package provider

import (
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
)

var (
	ErrProviderTimeout        = errors.New("notification provider call timed out")
	ErrProviderUnavailable    = errors.New("notification provider is unavailable")
	ErrDeliveryFailed         = errors.New("notification delivery failed")
	ErrInvalidRecipient       = errors.New("invalid notification recipient")
	ErrChannelNotSupported    = errors.New("notification channel is not supported")
	ErrProviderMisconfigured  = errors.New("notification provider configuration is invalid")
)

type Notification struct {
	ID        string            `json:"id"`
	EventID   string            `json:"event_id,omitempty"`
	UserID    string            `json:"user_id,omitempty"`
	Channel   string            `json:"channel"`
	Recipient string            `json:"recipient"`
	Subject   string            `json:"subject"`
	Content   string            `json:"content"`
	Metadata  map[string]string `json:"metadata,omitempty"`
}

type NotificationProvider interface {
	Send(ctx context.Context, notification Notification) error
	Channel() string
	ProviderName() string
}

type ProviderError struct {
	Channel      string
	ProviderName string
	Err          error
	Retryable    bool
}

func (e *ProviderError) Error() string {
	return fmt.Sprintf("provider error [%s/%s]: %v", e.Channel, e.ProviderName, e.Err)
}

func (e *ProviderError) Unwrap() error {
	return e.Err
}

func SanitizeError(err error, fallback error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) || errors.Is(err, context.Canceled) {
		return ErrProviderTimeout
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return ErrProviderTimeout
	}
	if errors.Is(err, ErrInvalidRecipient) || errors.Is(err, ErrChannelNotSupported) {
		return err
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline exceeded") {
		return ErrProviderTimeout
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") || strings.Contains(msg, "i/o timeout") {
		return ErrProviderUnavailable
	}
	if strings.Contains(msg, "invalid address") || strings.Contains(msg, "bad recipient") || strings.Contains(msg, "550") || strings.Contains(msg, "553") {
		return ErrInvalidRecipient
	}
	if fallback != nil {
		return fallback
	}
	return ErrDeliveryFailed
}

