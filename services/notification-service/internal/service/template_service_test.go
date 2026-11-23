package service

import (
	"context"
	"strings"
	"testing"
)

func TestTemplateService_ResolveFileBasedTemplate(t *testing.T) {
	// TemplateService initialized with no repository falls back to file-based templates
	svc := NewTemplateService()
	ctx := context.Background()

	tmpl, err := svc.ResolveTemplate(ctx, "UserRegistered", "")
	if err != nil {
		t.Fatalf("expected to resolve file template for UserRegistered: %v", err)
	}
	if tmpl.SubjectTemplate == "" || tmpl.BodyTemplate == "" {
		t.Fatalf("expected non-empty subject and body")
	}

	subject, body, err := svc.Render(tmpl, map[string]interface{}{
		"Email": "alice@example.com",
		"Role":  "CUSTOMER",
	})
	if err != nil {
		t.Fatalf("failed to render template: %v", err)
	}

	if subject != "Welcome to GoCart, alice@example.com!" {
		t.Errorf("unexpected rendered subject: %s", subject)
	}
	if !strings.Contains(body, "alice@example.com") || !strings.Contains(body, "GoCart") {
		t.Errorf("unexpected rendered body: %s", body)
	}
}

