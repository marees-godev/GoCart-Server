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

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
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

type testMailerClient struct {
	sent     []mailer.Message
	failNext error
}

func (m *testMailerClient) Send(ctx context.Context, msg mailer.Message) error {
	if m.failNext != nil {
		err := m.failNext
		m.failNext = nil
		return err
	}
	m.sent = append(m.sent, msg)
	return nil
}

func TestMailerEmailProvider_SendSuccess(t *testing.T) {
	client := &testMailerClient{}
	p := NewMailerEmailProvider(client, "notifications@gocart.local", 2*time.Second)

	if p.Channel() != "EMAIL" || p.ProviderName() != "mailer" {
		t.Fatalf("unexpected provider channel or name")
	}

	ctx := context.Background()
	notif := Notification{
		ID:        "job-email-1",
		Recipient: "customer@example.com",
		Subject:   "Order Update",
		Content:   "<h1>Your order has been placed</h1>",
		Channel:   "EMAIL",
	}

	err := p.Send(ctx, notif)
	if err != nil {
		t.Fatalf("expected MailerEmailProvider send success, got: %v", err)
	}

	if len(client.sent) != 1 {
		t.Fatalf("expected 1 sent email via mailer, got: %d", len(client.sent))
	}
	if client.sent[0].To != notif.Recipient || client.sent[0].Subject != notif.Subject {
		t.Errorf("sent message mismatch: %+v", client.sent[0])
	}
}

func TestMailerEmailProvider_InvalidRecipient(t *testing.T) {
	client := &testMailerClient{}
	p := NewMailerEmailProvider(client, "notifications@gocart.local", 2*time.Second)
	ctx := context.Background()

	testCases := []string{"", "   ", "invalid-no-at-sign"}
	for _, tc := range testCases {
		notif := Notification{
			ID:        "job-invalid",
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

func TestMailerEmailProvider_DeliveryFailure(t *testing.T) {
	client := &testMailerClient{
		failNext: errors.New("upstream service connection refused"),
	}
	p := NewMailerEmailProvider(client, "notifications@gocart.local", 2*time.Second)
	ctx := context.Background()

	notif := Notification{
		ID:        "job-fail",
		Recipient: "user@example.com",
		Subject:   "Subject",
		Content:   "Content",
		Channel:   "EMAIL",
	}

	err := p.Send(ctx, notif)
	if !errors.Is(err, ErrProviderUnavailable) {
		t.Fatalf("expected ErrProviderUnavailable, got: %v", err)
	}
}

func TestMailerEmailProvider_Timeout(t *testing.T) {
	client := &testMailerClient{}
	p := NewMailerEmailProvider(client, "notifications@gocart.local", 50*time.Millisecond)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // canceled context

	notif := Notification{
		ID:        "job-timeout",
		Recipient: "user@example.com",
		Subject:   "Subject",
		Content:   "Content",
		Channel:   "EMAIL",
	}

	err := p.Send(ctx, notif)
	if !errors.Is(err, ErrProviderTimeout) {
		t.Fatalf("expected ErrProviderTimeout, got: %v", err)
	}
}

func TestNewRegistryFromConfig(t *testing.T) {
	cfg := ProviderConfig{
		Email: EmailProviderConfig{
			Provider: "mock",
			Timeout:  1 * time.Second,
		},
	}

	reg := NewRegistryFromConfig(cfg, nil)

	emailP, err := reg.Get("EMAIL")
	if err != nil || emailP.Channel() != "EMAIL" {
		t.Fatalf("expected EMAIL provider in registry, got: %v", err)
	}

	// PUSH should not be supported (push removed, email alone)
	_, err = reg.Get("PUSH")
	if !errors.Is(err, ErrChannelNotSupported) {
		t.Fatalf("expected ErrChannelNotSupported for PUSH, got: %v", err)
	}

	// IN_APP should not be supported (email kept alone)
	_, err = reg.Get("IN_APP")
	if !errors.Is(err, ErrChannelNotSupported) {
		t.Fatalf("expected ErrChannelNotSupported for IN_APP, got: %v", err)
	}

	// SMS should not be supported
	_, err = reg.Get("SMS")
	if !errors.Is(err, ErrChannelNotSupported) {
		t.Fatalf("expected ErrChannelNotSupported for SMS, got: %v", err)
	}
}
