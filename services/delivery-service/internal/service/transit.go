package service

import (
	"strings"
	"time"
)

const (
	DeliveryTypeSameCity   = "Same-city delivery"
	DeliveryTypeSameState  = "Within the same state"
	DeliveryTypeInterState = "Inter-state delivery"

	TransitEstimateSameCity   = "1–2 days"
	TransitEstimateSameState  = "2–4 days"
	TransitEstimateInterState = "4–7 days"
)

type TransitRuleResult struct {
	RuleName         string
	DeliveryType     string
	EstimatedTransit string
	MinDays          int
	MaxDays          int
	TargetDate       time.Time
}

// CalculateTransitDeliveryDate computes SLA target delivery date based on Origin vs Destination:
// - Same-city delivery: 1–2 days (target: 2 days)
// - Within the same state: 2–4 days (target: 4 days)
// - Inter-state delivery: 4–7 days (target: 7 days)
func CalculateTransitDeliveryDate(originCity, originState, destCity, destState string, baseTime time.Time) TransitRuleResult {
	if baseTime.IsZero() {
		baseTime = time.Now().UTC()
	}
	oCity := strings.TrimSpace(strings.ToLower(originCity))
	dCity := strings.TrimSpace(strings.ToLower(destCity))
	oState := strings.TrimSpace(strings.ToLower(originState))
	dState := strings.TrimSpace(strings.ToLower(destState))

	// 1. Same-city delivery: 1–2 days (target: 2 days)
	if oCity != "" && dCity != "" && oCity == dCity {
		return TransitRuleResult{
			RuleName:         "Same-City",
			DeliveryType:     DeliveryTypeSameCity,
			EstimatedTransit: TransitEstimateSameCity,
			MinDays:          1,
			MaxDays:          2,
			TargetDate:       baseTime.Add(2 * 24 * time.Hour),
		}
	}

	// 2. Within the same state: 2–4 days (target: 4 days)
	if oState != "" && dState != "" && oState == dState {
		return TransitRuleResult{
			RuleName:         "Same-State",
			DeliveryType:     DeliveryTypeSameState,
			EstimatedTransit: TransitEstimateSameState,
			MinDays:          2,
			MaxDays:          4,
			TargetDate:       baseTime.Add(4 * 24 * time.Hour),
		}
	}

	// 3. Inter-state delivery: 4–7 days (target: 7 days)
	return TransitRuleResult{
		RuleName:         "Inter-State",
		DeliveryType:     DeliveryTypeInterState,
		EstimatedTransit: TransitEstimateInterState,
		MinDays:          4,
		MaxDays:          7,
		TargetDate:       baseTime.Add(7 * 24 * time.Hour),
	}
}
