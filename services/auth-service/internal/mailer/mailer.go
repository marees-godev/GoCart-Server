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
	"time"

	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
)

type Mailer interface {
	SendVerificationEmail(ctx context.Context, toEmail, token string) error
}

type fallbackMailer struct {
	cfg        config.EmailConfig
	httpClient *http.Client
	logger     *slog.Logger
}

func NewMailer(cfg config.EmailConfig, log *slog.Logger) Mailer {
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

func (m *fallbackMailer) SendVerificationEmail(ctx context.Context, toEmail, token string) error {
	verifyURL := fmt.Sprintf("%s/api/v1/auth/verify-email?token=%s", m.cfg.VerifyBaseURL, token)
	subject := "Verify your GoCart account email"
	bodyText := fmt.Sprintf("Welcome to GoCart! Please verify your email address by visiting the following link: %s\nThis link will expire in 30 minutes.", verifyURL)
	bodyHTML := fmt.Sprintf(`
		<div style="font-family: Arial, sans-serif; padding: 20px; color: #333;">
			<h2>Welcome to GoCart!</h2>
			<p>Thank you for registering. Please click the button below to verify your email address:</p>
			<p><a href="%s" style="background-color: #4F46E5; color: white; padding: 12px 24px; text-decoration: none; border-radius: 4px; display: inline-block;">Verify Email</a></p>
			<p style="font-size: 12px; color: #666;">Or copy and paste this link in your browser:<br><a href="%s">%s</a></p>
			<p style="font-size: 12px; color: #999;">This link expires in 30 minutes.</p>
		</div>
	`, verifyURL, verifyURL, verifyURL)

	// 1. Try Resend API (Primary)
	if m.cfg.ResendAPIKey != "" {
		if err := m.sendViaResend(ctx, toEmail, subject, bodyHTML, bodyText); err == nil {
			m.logger.Info("Verification email sent successfully via Resend", "to", toEmail)
			return nil
		} else {
			m.logger.Warn("Failed to send verification email via Resend, falling back to Brevo", "error", err)
		}
	}

	// 2. Try Brevo API (Secondary)
	if m.cfg.BrevoAPIKey != "" {
		if err := m.sendViaBrevo(ctx, toEmail, subject, bodyHTML, bodyText); err == nil {
			m.logger.Info("Verification email sent successfully via Brevo", "to", toEmail)
			return nil
		} else {
			m.logger.Warn("Failed to send verification email via Brevo, falling back to SMTP", "error", err)
		}
	}

	// 3. Try SMTP (Tertiary)
	if m.cfg.SMTPHost != "" && m.cfg.SMTPUser != "" {
		if err := m.sendViaSMTP(toEmail, subject, bodyHTML); err == nil {
			m.logger.Info("Verification email sent successfully via SMTP", "to", toEmail)
			return nil
		} else {
			m.logger.Warn("Failed to send verification email via SMTP", "error", err)
		}
	}

	m.logger.Warn("All email providers failed or unconfigured; verification link logged", "to", toEmail, "verify_url", verifyURL)
	return nil
}

func (m *fallbackMailer) sendViaResend(ctx context.Context, toEmail, subject, htmlBody, textBody string) error {
	fromEmail := m.cfg.ResendFromEmail
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}

	reqBody := map[string]any{
		"from":    fromEmail,
		"to":      []string{toEmail},
		"subject": subject,
		"html":    htmlBody,
		"text":    textBody,
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

func (m *fallbackMailer) sendViaBrevo(ctx context.Context, toEmail, subject, htmlBody, textBody string) error {
	fromEmail := m.cfg.BrevoFromEmail
	if fromEmail == "" {
		fromEmail = m.cfg.FromEmail
	}

	reqBody := map[string]any{
		"sender": map[string]string{
			"email": fromEmail,
			"name":  "GoCart",
		},
		"to": []map[string]string{
			{"email": toEmail},
		},
		"subject":     subject,
		"htmlContent": htmlBody,
		"textContent": textBody,
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

func (m *fallbackMailer) sendViaSMTP(toEmail, subject, htmlBody string) error {
	auth := smtp.PlainAuth("", m.cfg.SMTPUser, m.cfg.SMTPPass, m.cfg.SMTPHost)
	msg := fmt.Appendf(nil, "To: %s\r\nSubject: %s\r\nMIME-version: 1.0;\r\nContent-Type: text/html; charset=\"UTF-8\";\r\n\r\n%s", toEmail, subject, htmlBody)
	addr := fmt.Sprintf("%s:%s", m.cfg.SMTPHost, m.cfg.SMTPPort)
	return smtp.SendMail(addr, auth, m.cfg.FromEmail, []string{toEmail}, msg)
}
