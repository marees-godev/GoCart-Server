package provider

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"net/mail"
	"net/smtp"
	"strings"
	"time"
)

type SMTPConfig struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	UseTLS   bool
	Timeout  time.Duration
}

type SMTPProvider struct {
	config SMTPConfig
}

func NewSMTPProvider(cfg SMTPConfig) (*SMTPProvider, error) {
	if cfg.Host == "" {
		return nil, fmt.Errorf("%w: missing SMTP host", ErrProviderMisconfigured)
	}
	if cfg.Port <= 0 {
		cfg.Port = 587
	}
	if cfg.From == "" {
		cfg.From = "no-reply@gocart.local"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 10 * time.Second
	}
	return &SMTPProvider{
		config: cfg,
	}, nil
}

func (p *SMTPProvider) Channel() string {
	return "EMAIL"
}

func (p *SMTPProvider) ProviderName() string {
	return "smtp"
}

func (p *SMTPProvider) Send(ctx context.Context, notification Notification) error {
	ctx, cancel := context.WithTimeout(ctx, p.config.Timeout)
	defer cancel()

	recipient := strings.TrimSpace(notification.Recipient)
	if recipient == "" {
		return ErrInvalidRecipient
	}
	if _, err := mail.ParseAddress(recipient); err != nil && !strings.Contains(recipient, "@") {
		return ErrInvalidRecipient
	}

	addr := fmt.Sprintf("%s:%d", p.config.Host, p.config.Port)
	dialer := &net.Dialer{
		Timeout: p.config.Timeout,
	}

	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		return p.sanitizeError(err)
	}
	defer conn.Close()

	if dl, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(dl)
	}

	client, err := smtp.NewClient(conn, p.config.Host)
	if err != nil {
		return p.sanitizeError(err)
	}
	defer func() {
		_ = client.Quit()
	}()

	if p.config.UseTLS {
		tlsConfig := &tls.Config{
			ServerName: p.config.Host,
		}
		if ok, _ := client.Extension("STARTTLS"); ok {
			if err := client.StartTLS(tlsConfig); err != nil {
				return p.sanitizeError(err)
			}
		}
	}

	if p.config.Username != "" {
		auth := smtp.PlainAuth("", p.config.Username, p.config.Password, p.config.Host)
		if err := client.Auth(auth); err != nil {
			return p.sanitizeError(err)
		}
	}

	fromAddr := p.config.From
	if err := client.Mail(fromAddr); err != nil {
		return p.sanitizeError(err)
	}

	if err := client.Rcpt(recipient); err != nil {
		return p.sanitizeError(err)
	}

	w, err := client.Data()
	if err != nil {
		return p.sanitizeError(err)
	}

	subjectClean := strings.ReplaceAll(strings.ReplaceAll(notification.Subject, "\r", " "), "\n", " ")
	message := fmt.Sprintf("From: %s\r\nTo: %s\r\nSubject: %s\r\nMIME-Version: 1.0\r\nContent-Type: text/html; charset=UTF-8\r\n\r\n%s",
		fromAddr, recipient, subjectClean, notification.Content,
	)

	if _, err := w.Write([]byte(message)); err != nil {
		_ = w.Close()
		return p.sanitizeError(err)
	}

	if err := w.Close(); err != nil {
		return p.sanitizeError(err)
	}

	return nil
}

func (p *SMTPProvider) sanitizeError(err error) error {
	if err == nil {
		return nil
	}
	rawMsg := err.Error()
	if p.config.Password != "" {
		rawMsg = strings.ReplaceAll(rawMsg, p.config.Password, "******")
	}
	if p.config.Username != "" {
		rawMsg = strings.ReplaceAll(rawMsg, p.config.Username, "***")
	}

	sanitized := SanitizeError(err, ErrDeliveryFailed)
	if errors.Is(sanitized, ErrProviderTimeout) || errors.Is(sanitized, ErrProviderUnavailable) || errors.Is(sanitized, ErrInvalidRecipient) {
		return sanitized
	}

	return fmt.Errorf("%w: %s", ErrDeliveryFailed, rawMsg)
}
