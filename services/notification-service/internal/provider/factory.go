package provider

import (
	"log/slog"
	"strings"
	"time"
)

type ProviderConfig struct {
	Email EmailProviderConfig
	Push  PushConfig
}

type EmailProviderConfig struct {
	Provider string
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	Timeout  time.Duration
}

func NewRegistryFromConfig(cfg ProviderConfig, logger *slog.Logger) Registry {
	reg := NewRegistry()

	// 1. Email Channel Provider
	switch strings.ToLower(strings.TrimSpace(cfg.Email.Provider)) {
	case "smtp":
		smtpProvider, err := NewSMTPProvider(SMTPConfig{
			Host:     cfg.Email.Host,
			Port:     cfg.Email.Port,
			Username: cfg.Email.Username,
			Password: cfg.Email.Password,
			From:     cfg.Email.From,
			UseTLS:   cfg.Email.UseTLS,
			Timeout:  cfg.Email.Timeout,
		})
		if err != nil {
			if logger != nil {
				logger.Warn("Failed initializing SMTP email provider, falling back to mock provider", "error", err)
			}
			reg.Register("EMAIL", NewMockEmailProvider(cfg.Email.Timeout))
		} else {
			if logger != nil {
				logger.Info("Configured SMTP email provider", "host", cfg.Email.Host, "port", cfg.Email.Port, "from", cfg.Email.From)
			}
			reg.Register("EMAIL", smtpProvider)
		}
	case "mock":
		fallthrough
	default:
		reg.Register("EMAIL", NewMockEmailProvider(cfg.Email.Timeout))
	}

	// 2. Push Channel Provider
	switch strings.ToLower(strings.TrimSpace(cfg.Push.Provider)) {
	case "fcm":
		reg.Register("PUSH", NewFCMPushProvider(cfg.Push))
	case "mock":
		fallthrough
	default:
		reg.Register("PUSH", NewMockPushProvider(cfg.Push.Timeout))
	}

	// 3. In-App Channel Provider
	reg.Register("IN_APP", NewInAppProvider())

	return reg
}

