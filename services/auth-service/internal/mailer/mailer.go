package mailer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
)

type Mailer interface {
	SendVerificationEmail(ctx context.Context, toEmail, otp string) error
}

type authMailer struct {
	client mailer.Mailer
}

func NewMailer(cfg config.EmailConfig, log *slog.Logger) Mailer {
	client := mailer.New(cfg.ToMailerConfig(), log)
	return &authMailer{client: client}
}

func NewMailerWithClient(client mailer.Mailer) Mailer {
	return &authMailer{client: client}
}

func (m *authMailer) SendVerificationEmail(ctx context.Context, toEmail, otp string) error {
	subject := fmt.Sprintf("Your GoCart Verification Code: %s", otp)
	bodyText := fmt.Sprintf("Welcome to GoCart!\n\nYour email verification code is: %s\n\nThis code will expire in 5 minutes.", otp)
	bodyHTML := fmt.Sprintf(`
		<div style="font-family: -apple-system, BlinkMacSystemFont, 'Segoe UI', Roboto, Helvetica, Arial, sans-serif; max-width: 560px; margin: 0 auto; padding: 32px 24px; background-color: #ffffff; border: 1px solid #e2e8f0; border-radius: 12px;">
			<div style="margin-bottom: 24px;">
				<h1 style="color: #4F46E5; font-size: 24px; font-weight: 700; margin: 0 0 8px 0;">GoCart</h1>
				<h2 style="color: #1e293b; font-size: 20px; font-weight: 600; margin: 0;">Verify your email address</h2>
			</div>
			<p style="color: #475569; font-size: 15px; line-height: 1.6; margin: 0 0 24px 0;">
				Thank you for signing up with GoCart. Please use the following 6-digit verification code to complete your email verification:
			</p>
			<div style="background-color: #f1f5f9; border: 1px dashed #cbd5e1; border-radius: 8px; padding: 24px; text-align: center; margin-bottom: 24px;">
				<span style="font-family: 'Courier New', Courier, monospace; font-size: 36px; font-weight: 700; letter-spacing: 8px; color: #4F46E5; display: inline-block;">%s</span>
			</div>
			<p style="color: #64748b; font-size: 14px; line-height: 1.5; margin: 0;">
				This code is valid for 5 minutes. If you did not request this, please safely ignore this email.
			</p>
		</div>
	`, otp)

	return m.client.Send(ctx, mailer.Message{
		To:       toEmail,
		Subject:  subject,
		HTMLBody: bodyHTML,
		TextBody: bodyText,
	})
}
