package service

import (
	"context"
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
	})
	if err != nil {
		t.Fatalf("failed to render template: %v", err)
	}

	if subject != "Welcome to GoCart, alice@example.com!" {
		t.Errorf("unexpected rendered subject: %s", subject)
	}
	if body != "Hello alice@example.com, your account has been registered successfully on GoCart. Welcome aboard!" {
		t.Errorf("unexpected rendered body: %s", body)
	}
}

