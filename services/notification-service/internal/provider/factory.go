package provider

import (
	"log/slog"
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
	FromEmail       string
	Timeout         time.Duration
	ResendBaseURL   string
	BrevoBaseURL    string
}

type EmailProviderConfig = EmailConfig

func (c EmailConfig) ToMailerConfig() mailer.Config {
	return mailer.Config{
		ResendAPIKey:    c.ResendAPIKey,
		ResendFromEmail: c.ResendFromEmail,
		BrevoAPIKey:     c.BrevoAPIKey,
		BrevoFromEmail:  c.BrevoFromEmail,
		FromEmail:       c.FromEmail,
	}
}

func NewRegistryFromConfig(cfg ProviderConfig, logger *slog.Logger) Registry {
	reg := NewRegistry()

	// Email Channel Provider alone (Brevo and Resend alone)
	switch strings.ToLower(strings.TrimSpace(cfg.Email.Provider)) {
	case "mock":
		reg.Register("EMAIL", NewMockEmailProvider(cfg.Email.Timeout))
	default:
		// Default to Brevo and Resend alone
		emailProv := NewEmailProvider(cfg.Email, logger)
		reg.Register("EMAIL", emailProv)
		if logger != nil {
			logger.Info("Configured email provider with Brevo and Resend alone",
				"from_email", cfg.Email.FromEmail,
				"resend_enabled", cfg.Email.ResendAPIKey != "",
				"brevo_enabled", cfg.Email.BrevoAPIKey != "",
			)
		}
	}

	return reg
}
