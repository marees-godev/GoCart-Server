package provider

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
)

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
	replacementEmail := NewEmailProvider(EmailConfig{
		ResendAPIKey: "test-key",
		Timeout:      1 * time.Second,
	}, nil)
	reg.Register("EMAIL", replacementEmail)

	newP, _ := reg.Get("EMAIL")
	if newP.ProviderName() != "resend_brevo" {
		t.Fatalf("expected resend_brevo provider after replacement, got: %s", newP.ProviderName())
	}
}

func TestEmailProvider_ResendSuccess(t *testing.T) {
	resendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer re_valid_key" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"resend-123"}`))
	}))
	defer resendServer.Close()

	p := NewEmailProvider(EmailConfig{
		ResendAPIKey:  "re_valid_key",
		ResendBaseURL: resendServer.URL,
		Timeout:       2 * time.Second,
	}, nil)

	ctx := context.Background()
	err := p.Send(ctx, Notification{
		Recipient: "user@example.com",
		Subject:   "Order Delivered",
		Content:   "Your package arrived",
		Channel:   "EMAIL",
	})
	if err != nil {
		t.Fatalf("expected Resend send success, got: %v", err)
	}
}

func TestEmailProvider_ResendFails_BrevoFallbackSuccess(t *testing.T) {
	resendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":"resend rate limit"}`))
	}))
	defer resendServer.Close()

	brevoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("api-key") != "xkeysib-valid" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"messageId":"brevo-456"}`))
	}))
	defer brevoServer.Close()

	p := NewEmailProvider(EmailConfig{
		ResendAPIKey:  "re_rate_limited",
		ResendBaseURL: resendServer.URL,
		BrevoAPIKey:   "xkeysib-valid",
		BrevoBaseURL:  brevoServer.URL,
		Timeout:       2 * time.Second,
	}, nil)

	ctx := context.Background()
	err := p.Send(ctx, Notification{
		Recipient: "user@example.com",
		Subject:   "Fallback Test",
		Content:   "Testing Brevo fallback",
		Channel:   "EMAIL",
	})
	if err != nil {
		t.Fatalf("expected Brevo fallback success after Resend error, got: %v", err)
	}
}

func TestEmailProvider_BothFail(t *testing.T) {
	resendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer resendServer.Close()

	brevoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	defer brevoServer.Close()

	p := NewEmailProvider(EmailConfig{
		ResendAPIKey:  "re_failing",
		ResendBaseURL: resendServer.URL,
		BrevoAPIKey:   "xkeysib-failing",
		BrevoBaseURL:  brevoServer.URL,
		Timeout:       2 * time.Second,
	}, nil)

	ctx := context.Background()
	err := p.Send(ctx, Notification{
		Recipient: "user@example.com",
		Subject:   "Subject",
		Content:   "Body",
		Channel:   "EMAIL",
	})
	if !errors.Is(err, ErrDeliveryFailed) {
		t.Fatalf("expected ErrDeliveryFailed when both fail, got: %v", err)
	}
}

func TestEmailProvider_NeitherConfigured(t *testing.T) {
	p := NewEmailProvider(EmailConfig{
		Timeout: 2 * time.Second,
	}, nil)

	ctx := context.Background()
	err := p.Send(ctx, Notification{
		Recipient: "user@example.com",
		Subject:   "Subject",
		Content:   "Body",
		Channel:   "EMAIL",
	})
	if !errors.Is(err, ErrProviderMisconfigured) {
		t.Fatalf("expected ErrProviderMisconfigured, got: %v", err)
	}
}

func TestEmailProvider_InvalidRecipient(t *testing.T) {
	p := NewEmailProvider(EmailConfig{
		ResendAPIKey: "key",
		Timeout:      2 * time.Second,
	}, nil)

	err := p.Send(context.Background(), Notification{
		Recipient: "not-an-email",
		Subject:   "Subject",
		Content:   "Body",
	})
	if !errors.Is(err, ErrInvalidRecipient) {
		t.Fatalf("expected ErrInvalidRecipient, got: %v", err)
	}
}

func TestEmailProvider_Timeout(t *testing.T) {
	p := NewEmailProvider(EmailConfig{
		ResendAPIKey: "key",
		Timeout:      20 * time.Millisecond,
	}, nil)

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := p.Send(ctx, Notification{
		Recipient: "user@example.com",
		Subject:   "Subject",
		Content:   "Body",
	})
	if !errors.Is(err, ErrProviderTimeout) {
		t.Fatalf("expected ErrProviderTimeout, got: %v", err)
	}
}

func TestEmailProvider_APIKeysNeverLeaked(t *testing.T) {
	secretResendKey := "re_ultra_secret_key_12345"
	secretBrevoKey := "xkeysib_ultra_secret_key_67890"

	resendServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid key: " + secretResendKey))
	}))
	defer resendServer.Close()

	brevoServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("invalid key: " + secretBrevoKey))
	}))
	defer brevoServer.Close()

	p := NewEmailProvider(EmailConfig{
		ResendAPIKey:  secretResendKey,
		ResendBaseURL: resendServer.URL,
		BrevoAPIKey:   secretBrevoKey,
		BrevoBaseURL:  brevoServer.URL,
		Timeout:       2 * time.Second,
	}, nil)

	err := p.Send(context.Background(), Notification{
		Recipient: "user@example.com",
		Subject:   "Security Test",
		Content:   "Body",
	})
	if err == nil {
		t.Fatalf("expected error from failed authentication, got nil")
	}

	errStr := err.Error()
	if strings.Contains(errStr, secretResendKey) {
		t.Fatalf("SECURITY VIOLATION: Resend API key leaked in error message: %s", errStr)
	}
	if strings.Contains(errStr, secretBrevoKey) {
		t.Fatalf("SECURITY VIOLATION: Brevo API key leaked in error message: %s", errStr)
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

func TestEmailProvider_CustomClient(t *testing.T) {
	client := &testMailerClient{}
	p := NewEmailProviderWithClient(client, "notifications@gocart.local", 2*time.Second)

	err := p.Send(context.Background(), Notification{
		Recipient: "buyer@example.com",
		Subject:   "Welcome",
		Content:   "Hello",
		Channel:   "EMAIL",
	})
	if err != nil {
		t.Fatalf("expected send success, got: %v", err)
	}
	if len(client.sent) != 1 {
		t.Fatalf("expected 1 sent message, got %d", len(client.sent))
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
