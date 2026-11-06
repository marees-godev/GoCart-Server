package templates

import (
	"testing"
)

func TestGetTemplate(t *testing.T) {
	expectedEvents := []string{
		"UserRegistered",
		"StoreApproved",
		"StoreRejected",
		"OrderCreated",
		"PaymentSuccessful",
		"PaymentFailed",
		"OrderShipped",
		"OrderDelivered",
		"ReturnApproved",
		"ReturnRejected",
		"RefundCompleted",
	}

	for _, eventType := range expectedEvents {
		tmpl, ok := GetTemplate(eventType)
		if !ok || tmpl == nil {
			t.Errorf("expected template for event %s to exist in templates file", eventType)
			continue
		}
		if tmpl.SubjectTemplate == "" {
			t.Errorf("subject template for %s is empty", eventType)
		}
		if tmpl.BodyTemplate == "" {
			t.Errorf("body template for %s is empty", eventType)
		}
	}

	all := GetAllTemplates()
	if len(all) < len(expectedEvents) {
		t.Errorf("expected at least %d templates, got %d", len(expectedEvents), len(all))
	}
}

