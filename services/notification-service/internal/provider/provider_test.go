package provider

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"net"
	"strings"
	"testing"
	"time"
)

// Helper to spin up a local dummy SMTP server for testing SMTPProvider
func startTestSMTPServer(t *testing.T, shouldFailAuth bool) (string, func()) {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to start test smtp server: %v", err)
	}

	done := make(chan struct{})

	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-done:
					return
				default:
					return
				}
			}

			go handleSMTPConn(conn, shouldFailAuth)
		}
	}()

	teardown := func() {
		close(done)
		_ = listener.Close()
	}

	return listener.Addr().String(), teardown
}

func handleSMTPConn(conn net.Conn, shouldFailAuth bool) {
	defer conn.Close()
	reader := bufio.NewReader(conn)
	writer := bufio.NewWriter(conn)

	// 220 Service ready
	_, _ = writer.WriteString("220 test.smtp.local Simple Mail Transfer Service Ready\r\n")
	_ = writer.Flush()

	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		cmd := strings.ToUpper(strings.TrimSpace(line))

		if strings.HasPrefix(cmd, "EHLO") || strings.HasPrefix(cmd, "HELO") {
			_, _ = writer.WriteString("250-test.smtp.local\r\n250-AUTH PLAIN\r\n250 8BITMIME\r\n")
			_ = writer.Flush()
		} else if strings.HasPrefix(cmd, "AUTH") {
			if shouldFailAuth {
				_, _ = writer.WriteString("535 5.7.8 Authentication credentials invalid\r\n")
				_ = writer.Flush()
			} else {
				_, _ = writer.WriteString("235 2.7.0 Authentication successful\r\n")
				_ = writer.Flush()
			}
		} else if strings.HasPrefix(cmd, "MAIL FROM:") {
			_, _ = writer.WriteString("250 2.1.0 Ok\r\n")
			_ = writer.Flush()
		} else if strings.HasPrefix(cmd, "RCPT TO:") {
			if strings.Contains(cmd, "REJECT@EXAMPLE.COM") {
				_, _ = writer.WriteString("550 5.1.1 User unknown\r\n")
			} else {
				_, _ = writer.WriteString("250 2.1.5 Ok\r\n")
			}
			_ = writer.Flush()
		} else if strings.HasPrefix(cmd, "DATA") {
			_, _ = writer.WriteString("354 End data with <CR><LF>.<CR><LF>\r\n")
			_ = writer.Flush()

			for {
				dataLine, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if strings.TrimSpace(dataLine) == "." {
					break
				}
			}
			_, _ = writer.WriteString("250 2.0.0 Ok: queued\r\n")
			_ = writer.Flush()
		} else if strings.HasPrefix(cmd, "QUIT") {
			_, _ = writer.WriteString("221 2.0.0 Bye\r\n")
			_ = writer.Flush()
			return
		} else {
			_, _ = writer.WriteString("500 5.5.1 Unrecognized command\r\n")
			_ = writer.Flush()
		}
	}
}

func TestMockEmailProvider_SendSuccess(t *testing.T) {
	p := NewMockEmailProvider(2 * time.Second)
	ctx := context.Background()

	notif := Notification{
		ID:        "job-1",
		Recipient: "customer@example.com",
		Subject:   "Order Confirmed",
		Content:   "<h1>Your order #100 is confirmed</h1>",
		Channel:   "EMAIL",
	}

	err := p.Send(ctx, notif)
	if err != nil {
		t.Fatalf("expected send success, got error: %v", err)
	}

	sent := p.SentNotifications()
	if len(sent) != 1 {
		t.Fatalf("expected 1 sent notification, got: %d", len(sent))
	}
	if sent[0].Recipient != notif.Recipient || sent[0].Subject != notif.Subject {
		t.Errorf("sent notification payload mismatch: %+v", sent[0])
	}
}

func TestMockEmailProvider_InvalidRecipient(t *testing.T) {
	p := NewMockEmailProvider(2 * time.Second)
	ctx := context.Background()

	testCases := []string{"", "   ", "invalid-no-at-sign"}
	for _, tc := range testCases {
		notif := Notification{
			ID:        "job-inv",
			Recipient: tc,
			Subject:   "Subject",
			Content:   "Content",
			Channel:   "EMAIL",
		}
		err := p.Send(ctx, notif)
		if !errors.Is(err, ErrInvalidRecipient) {
			t.Errorf("expected ErrInvalidRecipient for recipient '%s', got: %v", tc, err)
		}
	}
}

func TestMockEmailProvider_ExplicitTimeout(t *testing.T) {
	p := NewMockEmailProvider(100 * time.Millisecond)
	p.SetLatency(250 * time.Millisecond)

	ctx := context.Background()
	notif := Notification{
		ID:        "job-timeout",
		Recipient: "delay@example.com",
		Subject:   "Delay",
		Content:   "Body",
		Channel:   "EMAIL",
	}

	err := p.Send(ctx, notif)
	if !errors.Is(err, ErrProviderTimeout) {
		t.Fatalf("expected ErrProviderTimeout, got: %v", err)
	}
}

func TestSMTPProvider_SendSuccess(t *testing.T) {
	addr, teardown := startTestSMTPServer(t, false)
	defer teardown()

	parts := strings.Split(addr, ":")
	host := parts[0]
	var port int
	_, _ = fmt.Sscanf(parts[1], "%d", &port)

	p, err := NewSMTPProvider(SMTPConfig{
		Host:     host,
		Port:     port,
		Username: "gocart-user",
		Password: "secret-password-123",
		From:     "notifications@gocart.local",
		Timeout:  3 * time.Second,
	})
	if err != nil {
		t.Fatalf("failed creating SMTP provider: %v", err)
	}

	ctx := context.Background()
	notif := Notification{
		ID:        "job-smtp-1",
		Recipient: "buyer@example.com",
		Subject:   "Welcome to GoCart",
		Content:   "<p>Welcome!</p>",
		Channel:   "EMAIL",
	}

	err = p.Send(ctx, notif)
	if err != nil {
		t.Fatalf("expected SMTP send success, got: %v", err)
	}
}

func TestSMTPProvider_InvalidRecipient(t *testing.T) {
	p, err := NewSMTPProvider(SMTPConfig{
		Host:    "localhost",
		Port:    25,
		Timeout: 2 * time.Second,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err = p.Send(context.Background(), Notification{
		Recipient: "",
		Subject:   "Hi",
		Content:   "Body",
	})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected ErrInvalidRecipient, got: %v", err)
	}
}

func TestSMTPProvider_ServerRejectsRecipient(t *testing.T) {
	addr, teardown := startTestSMTPServer(t, false)
	defer teardown()

	parts := strings.Split(addr, ":")
	var port int
	_, _ = fmt.Sscanf(parts[1], "%d", &port)

	p, _ := NewSMTPProvider(SMTPConfig{
		Host:    parts[0],
		Port:    port,
		Timeout: 3 * time.Second,
	})

	err := p.Send(context.Background(), Notification{
		Recipient: "reject@example.com",
		Subject:   "Test",
		Content:   "Test body",
	})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected ErrInvalidRecipient when server rejects rcpt, got: %v", err)
	}
}

func TestSMTPProvider_CredentialsNeverLoggedOrLeaked(t *testing.T) {
	addr, teardown := startTestSMTPServer(t, true) // will fail auth
	defer teardown()

	parts := strings.Split(addr, ":")
	var port int
	_, _ = fmt.Sscanf(parts[1], "%d", &port)

	secretPassword := "ultra-sensitive-smtp-password-999!"
	secretUsername := "super-secret-user"

	p, _ := NewSMTPProvider(SMTPConfig{
		Host:     parts[0],
		Port:     port,
		Username: secretUsername,
		Password: secretPassword,
		Timeout:  3 * time.Second,
	})

	err := p.Send(context.Background(), Notification{
		Recipient: "authfail@example.com",
		Subject:   "Auth Test",
		Content:   "Body",
	})

	if err == nil {
		t.Fatalf("expected auth failure, got nil")
	}

	errString := err.Error()
	if strings.Contains(errString, secretPassword) {
		t.Fatalf("SECURITY VIOLATION: Password leaked in error string: %s", errString)
	}
	if strings.Contains(errString, secretUsername) {
		t.Fatalf("SECURITY VIOLATION: Username leaked in error string: %s", errString)
	}
}

func TestSMTPProvider_ExplicitTimeout(t *testing.T) {
	// Connect to non-routable port or blackhole address to force timeout
	p, _ := NewSMTPProvider(SMTPConfig{
		Host:    "10.255.255.1",
		Port:    9999,
		Timeout: 50 * time.Millisecond,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := p.Send(ctx, Notification{
		Recipient: "timeout@example.com",
		Subject:   "Subject",
		Content:   "Body",
	})

	if !errors.Is(err, ErrProviderTimeout) && !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected timeout or unavailable error, got: %v", err)
	}
}

func TestProviderRegistry_RegistrationAndLookup(t *testing.T) {
	reg := NewRegistry()

	mockEmail := NewMockEmailProvider(1 * time.Second)
	reg.Register("EMAIL", mockEmail)

	// Case insensitivity
	p1, err := reg.Get("EMAIL")
	if err != nil || p1 != mockEmail {
		t.Fatalf("failed retrieving EMAIL provider: %v", err)
	}

	p2, err := reg.Get("email")
	if err != nil || p2 != mockEmail {
		t.Fatalf("failed retrieving email provider case-insensitively: %v", err)
	}

	// Unsupported channel
	_, err = reg.Get("UNKNOWN_CHANNEL")
	if !errors.Is(err, ErrChannelNotSupported) {
		t.Fatalf("expected ErrChannelNotSupported, got: %v", err)
	}
}

func TestProviderRegistry_SwappableImplementations(t *testing.T) {
	reg := NewRegistry()

	initialEmail := NewMockEmailProvider(1 * time.Second)
	reg.Register("EMAIL", initialEmail)

	p, _ := reg.Get("EMAIL")
	if p.ProviderName() != "mock-email" {
		t.Fatalf("expected mock-email, got: %s", p.ProviderName())
	}

	// Replace provider without modifying consumer or business logic
	replacementEmail, _ := NewSMTPProvider(SMTPConfig{
		Host:    "smtp.replaced.com",
		Port:    587,
		Timeout: 1 * time.Second,
	})
	reg.Register("EMAIL", replacementEmail)

	newP, _ := reg.Get("EMAIL")
	if newP.ProviderName() != "smtp" {
		t.Fatalf("expected smtp provider after replacement, got: %s", newP.ProviderName())
	}
}

func TestSMSAndPushProviders(t *testing.T) {
	ctx := context.Background()

	// SMS Mock Provider
	smsProv := NewMockSMSProvider(2 * time.Second)
	err := smsProv.Send(ctx, Notification{
		Recipient: "+1234567890",
		Content:   "Your OTP is 1234",
		Channel:   "SMS",
	})
	if err != nil {
		t.Fatalf("expected SMS send success, got: %v", err)
	}
	if len(smsProv.SentNotifications()) != 1 {
		t.Fatalf("expected 1 sent SMS, got %d", len(smsProv.SentNotifications()))
	}

	// SMS Invalid Recipient
	err = smsProv.Send(ctx, Notification{
		Recipient: "12",
		Content:   "OTP",
	})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected ErrInvalidRecipient for short phone, got: %v", err)
	}

	// Push Mock Provider
	pushProv := NewMockPushProvider(2 * time.Second)
	err = pushProv.Send(ctx, Notification{
		Recipient: "device-token-abc-123",
		Subject:   "Order Update",
		Content:   "Your order has shipped",
		Channel:   "PUSH",
	})
	if err != nil {
		t.Fatalf("expected Push send success, got: %v", err)
	}

	// Push Invalid Recipient
	err = pushProv.Send(ctx, Notification{
		Recipient: "",
		Content:   "Push content",
	})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected ErrInvalidRecipient for empty push recipient, got: %v", err)
	}
}

func TestNewRegistryFromConfig(t *testing.T) {
	cfg := ProviderConfig{
		Email: EmailProviderConfig{
			Provider: "mock",
			Timeout:  1 * time.Second,
		},
		SMS: SMSConfig{
			Provider: "mock",
			Timeout:  1 * time.Second,
		},
		Push: PushConfig{
			Provider: "mock",
			Timeout:  1 * time.Second,
		},
	}

	reg := NewRegistryFromConfig(cfg, nil)

	emailP, err := reg.Get("EMAIL")
	if err != nil || emailP.Channel() != "EMAIL" {
		t.Fatalf("expected EMAIL provider in registry, got: %v", err)
	}

	smsP, err := reg.Get("SMS")
	if err != nil || smsP.Channel() != "SMS" {
		t.Fatalf("expected SMS provider in registry, got: %v", err)
	}

	pushP, err := reg.Get("PUSH")
	if err != nil || pushP.Channel() != "PUSH" {
		t.Fatalf("expected PUSH provider in registry, got: %v", err)
	}

	inAppP, err := reg.Get("IN_APP")
	if err != nil || inAppP.Channel() != "IN_APP" {
		t.Fatalf("expected IN_APP provider in registry, got: %v", err)
	}
}
