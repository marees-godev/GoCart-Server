package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
)

type EmailProvider struct {
	cfg          EmailConfig
	httpClient   *http.Client
	logger       *slog.Logger
	customClient mailer.Mailer
}

type MailerEmailProvider = EmailProvider

// NewEmailProvider creates an email provider using Brevo and Resend alone.
// Resend API is attempted as primary, with Brevo API as fallback.
func NewEmailProvider(cfg EmailConfig, logger *slog.Logger) *EmailProvider {
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	if cfg.FromEmail == "" {
		cfg.FromEmail = "onboarding@resend.dev"
	}
	if logger == nil {
		logger = slog.Default()
	}
	return &EmailProvider{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: cfg.Timeout,
		},
		logger: logger,
	}
}

// NewEmailProviderWithClient allows injecting a custom mailer client for testing.
func NewEmailProviderWithClient(client mailer.Mailer, fromEmail string, timeout time.Duration) *EmailProvider {
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	if fromEmail == "" {
		fromEmail = "onboarding@resend.dev"
	}
	return &EmailProvider{
		cfg: EmailConfig{
			FromEmail: fromEmail,
			Timeout:   timeout,
		},
		httpClient:   &http.Client{Timeout: timeout},
		customClient: client,
		logger:       slog.Default(),
	}
}

// NewMailerEmailProvider provides backward compatibility for constructor references.
func NewMailerEmailProvider(client mailer.Mailer, fromEmail string, timeout time.Duration) *EmailProvider {
	return NewEmailProviderWithClient(client, fromEmail, timeout)
}

func (p *EmailProvider) Channel() string {
	return "EMAIL"
}

func (p *EmailProvider) ProviderName() string {
	return "resend_brevo"
}

func (p *EmailProvider) SetHTTPClient(client *http.Client) {
	if client != nil {
		p.httpClient = client
	}
}

func (p *EmailProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, p.cfg.Timeout)
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

	from := p.cfg.FromEmail
	if f, ok := notification.Metadata["from"]; ok && f != "" {
		from = f
	}

	if p.customClient != nil {
		msg := mailer.Message{
			To:       recipient,
			Subject:  notification.Subject,
			HTMLBody: notification.Content,
			TextBody: notification.Content,
			From:     from,
		}
		if err := p.customClient.Send(ctx, msg); err != nil {
			if ctx.Err() != nil {
				return ErrProviderTimeout
			}
			return SanitizeError(err, ErrDeliveryFailed)
		}
		return nil
	}

	if p.cfg.ResendAPIKey == "" && p.cfg.BrevoAPIKey == "" {
		return fmt.Errorf("%w: neither Resend nor Brevo API key is configured", ErrProviderMisconfigured)
	}

	// 1. Try Resend API (Primary)
	var resendErr error
	if p.cfg.ResendAPIKey != "" {
		resendErr = p.sendViaResend(ctx, recipient, notification.Subject, notification.Content, from)
		if resendErr == nil {
			if p.logger != nil {
				p.logger.InfoContext(ctx, "Email sent successfully via Resend", "to", recipient, "subject", notification.Subject)
			}
			return nil
		}
		if p.logger != nil {
			p.logger.WarnContext(ctx, "Failed to send email via Resend, falling back to Brevo", "error", resendErr)
		}
	}

	// 2. Try Brevo API (Secondary fallback)
	var brevoErr error
	if p.cfg.BrevoAPIKey != "" {
		brevoErr = p.sendViaBrevo(ctx, recipient, notification.Subject, notification.Content, from)
		if brevoErr == nil {
			if p.logger != nil {
				p.logger.InfoContext(ctx, "Email sent successfully via Brevo", "to", recipient, "subject", notification.Subject)
			}
			return nil
		}
		if p.logger != nil {
			p.logger.WarnContext(ctx, "Failed to send email via Brevo", "error", brevoErr)
		}
	}

	if ctx.Err() != nil {
		return ErrProviderTimeout
	}

	if resendErr != nil && brevoErr != nil {
		return fmt.Errorf("%w: resend failed (%v), brevo failed (%v)", ErrDeliveryFailed, p.sanitizeErr(resendErr), p.sanitizeErr(brevoErr))
	}
	if resendErr != nil {
		return fmt.Errorf("%w: resend failed: %v", ErrDeliveryFailed, p.sanitizeErr(resendErr))
	}
	return fmt.Errorf("%w: brevo failed: %v", ErrDeliveryFailed, p.sanitizeErr(brevoErr))
}

func (p *EmailProvider) sendViaResend(ctx context.Context, to, subject, content, from string) error {
	fromEmail := from
	if p.cfg.ResendFromEmail != "" {
		fromEmail = p.cfg.ResendFromEmail
	}

	reqBody := map[string]any{
		"from":    fromEmail,
		"to":      []string{to},
		"subject": subject,
		"html":    content,
		"text":    content,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	apiURL := "https://api.resend.com/emails"
	if p.cfg.ResendBaseURL != "" {
		apiURL = strings.TrimRight(p.cfg.ResendBaseURL, "/") + "/emails"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("resend status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p *EmailProvider) sendViaBrevo(ctx context.Context, to, subject, content, from string) error {
	fromEmail := from
	if p.cfg.BrevoFromEmail != "" {
		fromEmail = p.cfg.BrevoFromEmail
	}

	reqBody := map[string]any{
		"sender": map[string]string{
			"email": fromEmail,
			"name":  "GoCart",
		},
		"to": []map[string]string{
			{"email": to},
		},
		"subject":     subject,
		"htmlContent": content,
		"textContent": content,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	apiURL := "https://api.brevo.com/v3/smtp/email"
	if p.cfg.BrevoBaseURL != "" {
		apiURL = strings.TrimRight(p.cfg.BrevoBaseURL, "/") + "/v3/smtp/email"
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", p.cfg.BrevoAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("brevo status %d: %s", resp.StatusCode, string(b))
	}
	return nil
}

func (p *EmailProvider) sanitizeErr(err error) error {
	if err == nil {
		return nil
	}
	msg := err.Error()
	if p.cfg.ResendAPIKey != "" {
		msg = strings.ReplaceAll(msg, p.cfg.ResendAPIKey, "******")
	}
	if p.cfg.BrevoAPIKey != "" {
		msg = strings.ReplaceAll(msg, p.cfg.BrevoAPIKey, "******")
	}
	return errors.New(msg)
}
