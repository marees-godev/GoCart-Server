package mailer_test

import (
	"context"
	"testing"

	"github.com/marees-godev/GoCart-Server/pkg/mailer"
)

func TestMailerFallbackWhenUnconfigured(t *testing.T) {
	// Unconfigured mailer logs warning and returns nil
	client := mailer.New(mailer.Config{}, nil)
	err := client.Send(context.Background(), mailer.Message{
		To:       "user@example.com",
		Subject:  "Test Subject",
		HTMLBody: "<p>Hello</p>",
		TextBody: "Hello",
	})
	if err != nil {
		t.Fatalf("expected nil error on unconfigured fallback, got: %v", err)
	}
}
