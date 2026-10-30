package service

import (
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

func TestCalculateTransitDeliveryDate(t *testing.T) {
	baseTime := time.Date(2026, 10, 15, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name         string
		originCity   string
		originState  string
		destCity     string
		destState    string
		expectedRule string
		expectedDays int
	}{
		{
			name:         "Same City",
			originCity:   "Seattle",
			originState:  "WA",
			destCity:     "Seattle",
			destState:    "WA",
			expectedRule: "Same-City",
			expectedDays: 2,
		},
		{
			name:         "Same State Different City",
			originCity:   "Seattle",
			originState:  "WA",
			destCity:     "Spokane",
			destState:    "WA",
			expectedRule: "Same-State",
			expectedDays: 4,
		},
		{
			name:         "Inter State",
			originCity:   "Seattle",
			originState:  "WA",
			destCity:     "Portland",
			destState:    "OR",
			expectedRule: "Inter-State",
			expectedDays: 7,
		},
		{
			name:         "Case Insensitive Same City",
			originCity:   "NEW YORK",
			originState:  "NY",
			destCity:     "new york",
			destState:    "ny",
			expectedRule: "Same-City",
			expectedDays: 2,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			res := CalculateTransitDeliveryDate(tc.originCity, tc.originState, tc.destCity, tc.destState, baseTime)
			if res.RuleName != tc.expectedRule {
				t.Errorf("expected rule %s, got %s", tc.expectedRule, res.RuleName)
			}
			expectedDate := baseTime.Add(time.Duration(tc.expectedDays) * 24 * time.Hour)
			if !res.TargetDate.Equal(expectedDate) {
				t.Errorf("expected date %v, got %v", expectedDate, res.TargetDate)
			}
		})
	}
}

func TestResolveProvisionalOrigin(t *testing.T) {
	warehouses := []*model.Warehouse{
		{City: "Seattle", State: "WA"},
		{City: "Los Angeles", State: "CA"},
		{City: "Dallas", State: "TX"},
	}

	t.Run("empty list returns empty strings", func(t *testing.T) {
		city, state := resolveProvisionalOrigin(nil, "Seattle", "WA")
		if city != "" || state != "" {
			t.Errorf("expected empty origin, got %s, %s", city, state)
		}
	})

	t.Run("city match selects specific warehouse", func(t *testing.T) {
		city, state := resolveProvisionalOrigin(warehouses, "los angeles", "ca")
		if city != "Los Angeles" || state != "CA" {
			t.Errorf("expected Los Angeles, CA, got %s, %s", city, state)
		}
	})

	t.Run("state match selects warehouse in same state", func(t *testing.T) {
		city, state := resolveProvisionalOrigin(warehouses, "Houston", "TX")
		if city != "Dallas" || state != "TX" {
			t.Errorf("expected Dallas, TX, got %s, %s", city, state)
		}
	})

	t.Run("no proximity match defaults to first warehouse as provisional origin", func(t *testing.T) {
		city, state := resolveProvisionalOrigin(warehouses, "Miami", "FL")
		if city != "Seattle" || state != "WA" {
			t.Errorf("expected Seattle, WA, got %s, %s", city, state)
		}
	})
}
