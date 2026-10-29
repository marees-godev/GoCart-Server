package model

import (
	"testing"
)

func TestParseShippingAddress(t *testing.T) {
	t.Run("parses comma-separated street string with city", func(t *testing.T) {
		raw := "24 satya sai nagar , tvs nagar ,madurai"
		addr := ParseShippingAddress(raw)
		if addr.Street != raw {
			t.Errorf("expected street %q, got %q", raw, addr.Street)
		}
		if addr.City != "madurai" {
			t.Errorf("expected city 'madurai', got %q", addr.City)
		}
	})

	t.Run("parses comma-separated string with city, state, postal code", func(t *testing.T) {
		raw := "24 satya sai nagar, tvs nagar, madurai, tamil nadu, 625003"
		addr := ParseShippingAddress(raw)
		if addr.City != "madurai" {
			t.Errorf("expected city 'madurai', got %q", addr.City)
		}
		if addr.State != "Tamil Nadu" {
			t.Errorf("expected state 'Tamil Nadu', got %q", addr.State)
		}
		if addr.PostalCode != "625003" {
			t.Errorf("expected postal code '625003', got %q", addr.PostalCode)
		}
	})

	t.Run("parses wrapped JSON from order service", func(t *testing.T) {
		raw := `{"address": "24 satya sai nagar , tvs nagar ,madurai", "contact_number": "6383449176"}`
		addr := ParseShippingAddress(raw)
		if addr.Street != "24 satya sai nagar , tvs nagar ,madurai" {
			t.Errorf("expected street unpacked, got %q", addr.Street)
		}
		if addr.City != "madurai" {
			t.Errorf("expected city 'madurai', got %q", addr.City)
		}
		if addr.ContactNumber != "6383449176" {
			t.Errorf("expected contact number '6383449176', got %q", addr.ContactNumber)
		}
	})

	t.Run("parses standard full JSON address", func(t *testing.T) {
		raw := `{"street": "123 Main St", "city": "Seattle", "state": "WA", "postal_code": "98101", "country": "USA"}`
		addr := ParseShippingAddress(raw)
		if addr.Street != "123 Main St" || addr.City != "Seattle" || addr.State != "WA" || addr.PostalCode != "98101" {
			t.Errorf("unexpected parsed address: %+v", addr)
		}
	})
}

func TestInferCityStateFromWarehouses(t *testing.T) {
	warehouses := []*Warehouse{
		{City: "Madurai", State: "Tamil Nadu"},
		{City: "Chennai", State: "Tamil Nadu"},
		{City: "Seattle", State: "WA"},
	}

	t.Run("infers city and state matching warehouse in address street", func(t *testing.T) {
		addr := ShippingAddress{
			Street: "24 satya sai nagar , tvs nagar ,madurai",
		}
		InferCityStateFromWarehouses(&addr, warehouses)
		if addr.City != "Madurai" {
			t.Errorf("expected city 'Madurai', got %q", addr.City)
		}
		if addr.State != "Tamil Nadu" {
			t.Errorf("expected state 'Tamil Nadu', got %q", addr.State)
		}
	})
}

