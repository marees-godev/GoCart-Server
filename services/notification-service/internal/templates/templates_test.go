package templates

import (
	"strings"
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
		"OrderPaymentFailed",
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

	userRegTmpl, ok := GetTemplate("UserRegistered", "EMAIL")
	if !ok || userRegTmpl == nil {
		t.Fatalf("expected UserRegistered EMAIL template to be loaded")
	}
	if userRegTmpl.BodyTemplate == "" {
		t.Fatalf("expected UserRegistered BodyTemplate to not be empty")
	}
	if !strings.Contains(userRegTmpl.BodyTemplate, "<!DOCTYPE html>") {
		t.Errorf("expected UserRegistered EMAIL template to load HTML file, got: %s", userRegTmpl.BodyTemplate)
	}

	orderEvents := []string{
		"OrderCreated",
		"OrderConfirmed",
		"OrderShipped",
		"OrderDelivered",
		"OrderCancelled",
	}
	for _, evt := range orderEvents {
		tmpl, ok := GetTemplate(evt, "EMAIL")
		if !ok || tmpl == nil {
			t.Errorf("expected %s EMAIL template to be loaded", evt)
			continue
		}
		if !strings.Contains(tmpl.BodyTemplate, "<!DOCTYPE html>") {
			t.Errorf("expected %s EMAIL template to load HTML file, got: %s", evt, tmpl.BodyTemplate)
		}
		if !strings.Contains(tmpl.BodyTemplate, "GoCart Order Notification") {
			t.Errorf("expected %s to have GoCart Order Notification title", evt)
		}
	}

	merchantEvents := []string{
		"MerchantApproved",
		"MerchantActivated",
		"MerchantRejected",
		"MerchantSuspended",
	}
	for _, evt := range merchantEvents {
		tmpl, ok := GetTemplate(evt, "EMAIL")
		if !ok || tmpl == nil {
			t.Errorf("expected %s EMAIL template to be loaded", evt)
			continue
		}
		if !strings.Contains(tmpl.BodyTemplate, "<!DOCTYPE html>") {
			t.Errorf("expected %s EMAIL template to load HTML file, got: %s", evt, tmpl.BodyTemplate)
		}
		if !strings.Contains(tmpl.BodyTemplate, "GoCart Merchant Notification") {
			t.Errorf("expected %s to have GoCart Merchant Notification title", evt)
		}
	}

	storeEvents := []string{
		"StoreApproved",
		"StoreRejected",
	}
	for _, evt := range storeEvents {
		tmpl, ok := GetTemplate(evt, "EMAIL")
		if !ok || tmpl == nil {
			t.Errorf("expected %s EMAIL template to be loaded", evt)
			continue
		}
		if !strings.Contains(tmpl.BodyTemplate, "<!DOCTYPE html>") {
			t.Errorf("expected %s EMAIL template to load HTML file, got: %s", evt, tmpl.BodyTemplate)
		}
		if !strings.Contains(tmpl.BodyTemplate, "GoCart Store Notification") {
			t.Errorf("expected %s to have GoCart Store Notification title", evt)
		}
	}

	paymentEvents := []string{
		"PaymentSuccessful",
		"PaymentFailed",
		"OrderPaymentFailed",
		"RefundCompleted",
	}
	for _, evt := range paymentEvents {
		tmpl, ok := GetTemplate(evt, "EMAIL")
		if !ok || tmpl == nil {
			t.Errorf("expected %s EMAIL template to be loaded", evt)
			continue
		}
		if !strings.Contains(tmpl.BodyTemplate, "<!DOCTYPE html>") {
			t.Errorf("expected %s EMAIL template to load HTML file, got: %s", evt, tmpl.BodyTemplate)
		}
		if !strings.Contains(tmpl.BodyTemplate, "GoCart Payment Notification") {
			t.Errorf("expected %s to have GoCart Payment Notification title", evt)
		}
	}

	returnEvents := []string{
		"ReturnApproved",
		"ReturnRejected",
	}
	for _, evt := range returnEvents {
		tmpl, ok := GetTemplate(evt, "EMAIL")
		if !ok || tmpl == nil {
			t.Errorf("expected %s EMAIL template to be loaded", evt)
			continue
		}
		if !strings.Contains(tmpl.BodyTemplate, "<!DOCTYPE html>") {
			t.Errorf("expected %s EMAIL template to load HTML file, got: %s", evt, tmpl.BodyTemplate)
		}
		if !strings.Contains(tmpl.BodyTemplate, "GoCart Return Notification") {
			t.Errorf("expected %s to have GoCart Return Notification title", evt)
		}
	}
}
