package mailer

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/smtp"
	"strings"
	"time"
)

type Message struct {
	To       string
	Subject  string
	HTMLBody string
	TextBody string
	From     string
}

type Mailer interface {
	Send(ctx context.Context, msg Message) error
}

type fallbackMailer struct {
	cfg        Config
	httpClient *http.Client
	logger     *slog.Logger
}

func New(cfg Config, log *slog.Logger) Mailer {
	if log == nil {
		log = slog.Default()
	}
	return &fallbackMailer{
		cfg: cfg,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		logger: log,
	}
}

func (m *fallbackMailer) Send(ctx context.Context, msg Message) error {
	msg.To = strings.ToLower(strings.TrimSpace(msg.To))

	// 1. Try Resend API (Primary)
	if m.cfg.ResendAPIKey != "" {
		if err := m.sendViaResend(ctx, msg); err == nil {
			m.logger.Info("Email sent successfully via Resend", "to", msg.To, "subject", msg.Subject)
			return nil
		} else {
			m.logger.Warn("Failed to send email via Resend, falling back to Brevo", "error", err)
		}
	}

	// 2. Try Brevo API (Secondary)
	if m.cfg.BrevoAPIKey != "" {
		if err := m.sendViaBrevo(ctx, msg); err == nil {
			m.logger.Info("Email sent successfully via Brevo", "to", msg.To, "subject", msg.Subject)
			return nil
		} else {
			m.logger.Warn("Failed to send email via Brevo, falling back to SMTP", "error", err)
		}
	}

	// 3. Try SMTP (Tertiary)
	if m.cfg.SMTPHost != "" && m.cfg.SMTPUser != "" {
		if err := m.sendViaSMTP(msg); err == nil {
			m.logger.Info("Email sent successfully via SMTP", "to", msg.To, "subject", msg.Subject)
			return nil
		} else {
			m.logger.Warn("Failed to send email via SMTP", "error", err)
		}
	}

	m.logger.Warn("All email providers failed or unconfigured", "to", msg.To, "subject", msg.Subject)
	return nil
}

func (m *fallbackMailer) sendViaResend(ctx context.Context, msg Message) error {
	fromEmail := msg.From
	if fromEmail == "" {
		fromEmail = m.cfg.ResendFromEmail
	}
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}

	reqBody := map[string]any{
		"from":    fromEmail,
		"to":      []string{msg.To},
		"subject": msg.Subject,
		"html":    msg.HTMLBody,
		"text":    msg.TextBody,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.resend.com/emails", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+m.cfg.ResendAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
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

func (m *fallbackMailer) sendViaBrevo(ctx context.Context, msg Message) error {
	fromEmail := msg.From
	if fromEmail == "" {
		fromEmail = m.cfg.BrevoFromEmail
	}
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}

	reqBody := map[string]any{
		"sender": map[string]string{
			"email": fromEmail,
			"name":  "GoCart",
		},
		"to": []map[string]string{
			{"email": msg.To},
		},
		"subject":     msg.Subject,
		"htmlContent": msg.HTMLBody,
		"textContent": msg.TextBody,
	}

	jsonData, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", "https://api.brevo.com/v3/smtp/email", bytes.NewBuffer(jsonData))
	if err != nil {
		return err
	}
	req.Header.Set("api-key", m.cfg.BrevoAPIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := m.httpClient.Do(req)
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

func (m *fallbackMailer) sendViaSMTP(msg Message) error {
	fromEmail := msg.From
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	mimeMsg := fmt.Appendf(nil, "To: %s\r\nSubject: %s\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n%s", msg.To, msg.Subject, msg.HTMLBody)
	addr := fmt.Sprintf("%s:%s", m.cfg.SMTPHost, m.cfg.SMTPPort)
	return smtp.SendMail(addr, auth, fromEmail, []string{msg.To}, mimeMsg)
}
