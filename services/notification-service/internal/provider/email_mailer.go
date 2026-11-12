package provider

import (
	"context"
	"net/mail"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
)

type MailerEmailProvider struct {
	client    mailer.Mailer
	fromEmail string
	timeout   time.Duration
}

// NewMailerEmailProvider creates an email provider backed by pkg/mailer.
// pkg/mailer delivers via Resend API (Primary) -> Brevo API (Secondary) -> SMTP (Tertiary) fallback.
func NewMailerEmailProvider(client mailer.Mailer, fromEmail string, timeout time.Duration) *MailerEmailProvider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if fromEmail == "" {
		fromEmail = "onboarding@resend.dev"
	}
	return &MailerEmailProvider{
		client:    client,
		fromEmail: fromEmail,
		timeout:   timeout,
	}
}

// NewEmailProvider is an alias for NewMailerEmailProvider
func NewEmailProvider(client mailer.Mailer, fromEmail string, timeout time.Duration) *MailerEmailProvider {
	return NewMailerEmailProvider(client, fromEmail, timeout)
}

func (p *MailerEmailProvider) Channel() string {
	return "EMAIL"
}

func (p *MailerEmailProvider) ProviderName() string {
	return "mailer"
}

func (p *MailerEmailProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, p.timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
		return ErrInvalidRecipient
	}
	if _, err := mail.ParseAddress(recipient); err != nil && !strings.Contains(recipient, "@") {
		return ErrInvalidRecipient
	}

	select {
	case <-ctx.Done():
		return ErrProviderTimeout
	default:
	}

	from := p.fromEmail
	if f, ok := notification.Metadata["from"]; ok && f != "" {
		from = f
	}

	msg := mailer.Message{
		To:       recipient,
		Subject:  notification.Subject,
		HTMLBody: notification.Content,
		TextBody: notification.Content,
		From:     from,
	}

	if err := p.client.Send(ctx, msg); err != nil {
		if ctx.Err() != nil {
			return ErrProviderTimeout
		}
		return SanitizeError(err, ErrDeliveryFailed)
	}

	if ctx.Err() != nil {
		return ErrProviderTimeout
	}

	return nil
}

