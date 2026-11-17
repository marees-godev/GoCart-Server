package templates

import (
	"testing"
)

func TestGetTemplate(t *testing.T) {
	expectedEvents := []string{
		"UserRegistered",
		"StoreApproved",
		"StoreRejected",
		"MerchantApproved",
		"MerchantActivated",
		"MerchantRejected",
		"MerchantSuspended",
		"OrderCreated",
		"OrderConfirmed",
		"OrderCancelled",
		"PaymentSuccessful",
		"PaymentFailed",
		"OrderShipped",
		"OrderDelivered",
		"DeliveryCreated",
		"DeliveryStatusUpdated",
		"DeliveryStatusChanged",
		"ReturnApproved",
		"ReturnRejected",
		"RefundCompleted",
	}

	for _, eventType := range expectedEvents {
		tmpl, ok := GetTemplate(eventType)
		if !ok || tmpl == nil {
			t.Errorf("expected default template for event %s to exist in templates file", eventType)
			continue
		}
		if tmpl.SubjectTemplate == "" {
			t.Errorf("subject template for %s is empty", eventType)
		}
		if tmpl.BodyTemplate == "" {
			t.Errorf("body template for %s is empty", eventType)
		}

		pushTmpl, pushOk := GetTemplate(eventType, "PUSH")
		if !pushOk || pushTmpl == nil {
			t.Errorf("expected PUSH template for event %s to exist in templates file", eventType)
			continue
		}
		if pushTmpl.SubjectTemplate == "" || pushTmpl.BodyTemplate == "" {
			t.Errorf("PUSH template fields empty for %s", eventType)
		}
	}

	pushEvents := []string{
		"OrderCreated",
		"DeliveryCreated",
	}
	for _, eventType := range pushEvents {
		pushTmpl, pushOk := GetTemplate(eventType, "PUSH")
		if !pushOk || pushTmpl == nil {
			t.Errorf("expected PUSH template for event %s to exist in templates file", eventType)
			continue
		}
		if pushTmpl.SubjectTemplate == "" || pushTmpl.BodyTemplate == "" {
			t.Errorf("PUSH template fields empty for %s", eventType)
		}
	}

	all := GetAllTemplates()
	if len(all) < len(expectedEvents) {
		t.Errorf("expected at least %d templates, got %d", len(expectedEvents), len(all))
	}
}
