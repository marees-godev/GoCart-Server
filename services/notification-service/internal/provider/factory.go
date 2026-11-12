package provider

import (
	"log/slog"
	"strconv"
	"strings"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
)

type ProviderConfig struct {
	Email EmailConfig
}

type EmailConfig struct {
	Provider        string
	ResendAPIKey    string
	ResendFromEmail string
	BrevoAPIKey     string
	BrevoFromEmail  string
	SMTPHost        string
	SMTPPort        string
	SMTPUser        string
	SMTPPass        string
	FromEmail       string
	Timeout         time.Duration
}

type EmailProviderConfig = EmailConfig

func (c EmailConfig) ToMailerConfig() mailer.Config {
	return mailer.Config{
		ResendAPIKey:    c.ResendAPIKey,
		ResendFromEmail: c.ResendFromEmail,
		BrevoAPIKey:     c.BrevoAPIKey,
		BrevoFromEmail:  c.BrevoFromEmail,
		SMTPHost:        c.SMTPHost,
		SMTPPort:        c.SMTPPort,
		SMTPUser:        c.SMTPUser,
		SMTPPass:        c.SMTPPass,
		FromEmail:       c.FromEmail,
	}
}

func NewRegistryFromConfig(cfg ProviderConfig, logger *slog.Logger) Registry {
	reg := NewRegistry()

	// Email Channel Provider alone (Brevo, Resend, and SMTP fallback like auth-service)
	switch strings.ToLower(strings.TrimSpace(cfg.Email.Provider)) {
	case "mock":
		reg.Register("EMAIL", NewMockEmailProvider(cfg.Email.Timeout))
	case "smtp":
		port := 587
		if cfg.Email.SMTPPort != "" {
			if p, err := strconv.Atoi(cfg.Email.SMTPPort); err == nil && p > 0 {
				port = p
			}
		}
		smtpProvider, err := NewSMTPProvider(SMTPConfig{
			Host:     cfg.Email.SMTPHost,
			Port:     port,
			Username: cfg.Email.SMTPUser,
			Password: cfg.Email.SMTPPass,
			From:     cfg.Email.FromEmail,
			Timeout:  cfg.Email.Timeout,
		})
		if err != nil {
			if logger != nil {
				logger.Warn("Failed initializing direct SMTP email provider, falling back to multi-provider mailer", "error", err)
			}
			mailerClient := mailer.New(cfg.Email.ToMailerConfig(), logger)
			reg.Register("EMAIL", NewMailerEmailProvider(mailerClient, cfg.Email.FromEmail, cfg.Email.Timeout))
		} else {
			if logger != nil {
				logger.Info("Configured direct SMTP email provider", "host", cfg.Email.SMTPHost, "port", port, "from", cfg.Email.FromEmail)
			}
			reg.Register("EMAIL", smtpProvider)
		}
	default:
		// Default to multi-provider mailer using Resend -> Brevo -> SMTP fallback (matching auth-service)
		mailerClient := mailer.New(cfg.Email.ToMailerConfig(), logger)
		reg.Register("EMAIL", NewMailerEmailProvider(mailerClient, cfg.Email.FromEmail, cfg.Email.Timeout))
		if logger != nil {
			logger.Info("Configured email provider with Resend, Brevo, and SMTP fallback",
				"from_email", cfg.Email.FromEmail,
				"resend_enabled", cfg.Email.ResendAPIKey != "",
				"brevo_enabled", cfg.Email.BrevoAPIKey != "",
				"smtp_configured", cfg.Email.SMTPHost != "" && cfg.Email.SMTPUser != "",
			)
		}
	}

	return reg
}
