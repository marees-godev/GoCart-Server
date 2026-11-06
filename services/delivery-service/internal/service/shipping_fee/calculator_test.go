package shipping_fee

import (
	"context"
	"testing"
)

func TestShippingFeeCalculation_WeightSlabs(t *testing.T) {
	calc := NewWeightBracketCalculator("INR")
	ctx := context.Background()

	tests := []struct {
		name        string
		weight      float64
		zone        string
		expectedFee float64
	}{
		// 0-500g slab
		{"0-500g IntraCity", 300, ZoneIntraCity, 40.0},
		{"0-500g TamilNadu", 500, ZoneTamilNadu, 55.0},
		{"0-500g Regional", 100, ZoneRegional, 65.0},

		// 501g-1kg slab
		{"501g-1kg IntraCity", 501, ZoneIntraCity, 50.0},
		{"501g-1kg TamilNadu", 1000, ZoneTamilNadu, 70.0},
		{"501g-1kg Regional", 800, ZoneRegional, 90.0},

		// 1-2kg slab
		{"1-2kg IntraCity", 1001, ZoneIntraCity, 65.0},
		{"1-2kg TamilNadu", 1500, ZoneTamilNadu, 90.0},
		{"1-2kg Regional", 2000, ZoneRegional, 120.0},

		// 2-3kg slab
		{"2-3kg IntraCity", 2001, ZoneIntraCity, 85.0},
		{"2-3kg TamilNadu", 2500, ZoneTamilNadu, 120.0},
		{"2-3kg Regional", 3000, ZoneRegional, 160.0},

		// 3-5kg slab
		{"3-5kg IntraCity", 3001, ZoneIntraCity, 120.0},
		{"3-5kg TamilNadu", 4500, ZoneTamilNadu, 170.0},
		{"3-5kg Regional", 5000, ZoneRegional, 230.0},

		// Additional kg slab (> 5000g)
		// 5001g: 1 additional kg
		{"5001g IntraCity (1 add kg)", 5001, ZoneIntraCity, 120.0 + 25.0},
		{"5001g TamilNadu (1 add kg)", 5001, ZoneTamilNadu, 170.0 + 35.0},
		{"5001g Regional (1 add kg)", 5001, ZoneRegional, 230.0 + 50.0},

		// 6500g: 2 additional kg
		{"6500g IntraCity (2 add kg)", 6500, ZoneIntraCity, 120.0 + 2*25.0},
		{"6500g TamilNadu (2 add kg)", 6500, ZoneTamilNadu, 170.0 + 2*35.0},
		{"6500g Regional (2 add kg)", 6500, ZoneRegional, 230.0 + 2*50.0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := calc.Calculate(ctx, &CalculationRequest{
				WeightGrams: tt.weight,
				Zone:        tt.zone,
			})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.ShippingFee != tt.expectedFee {
				t.Errorf("expected fee %.2f, got %.2f", tt.expectedFee, res.ShippingFee)
			}
		})
	}
}

func TestShippingFeeCalculation_ItemsWeight(t *testing.T) {
	calc := NewWeightBracketCalculator("INR")
	ctx := context.Background()

	res, err := calc.Calculate(ctx, &CalculationRequest{
		Zone: ZoneIntraCity,
		Items: []CalculationItem{
			{ProductID: "prod-1", Quantity: 2, WeightGrams: 300}, // 600g
			{ProductID: "prod-2", Quantity: 1, WeightGrams: 500}, // 500g => total 1100g (1-2kg)
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// 1100g in IntraCity is 1-2kg slab: ₹65
	if res.ShippingFee != 65.0 {
		t.Errorf("expected 65.0, got %.2f", res.ShippingFee)
	}
	if res.TotalWeightGrams != 1100.0 {
		t.Errorf("expected total weight 1100.0, got %.2f", res.TotalWeightGrams)
	}
}

func TestShippingFeeCalculation_ZoneResolution(t *testing.T) {
	calc := NewWeightBracketCalculator("INR")
	ctx := context.Background()

	// Same city -> IntraCity
	res, _ := calc.Calculate(ctx, &CalculationRequest{
		WeightGrams:      400,
		OriginCity:       "Chennai",
		DestinationCity:  "Chennai",
		OriginState:      "Tamil Nadu",
		DestinationState: "Tamil Nadu",
	})
	if res.Zone != ZoneIntraCity || res.ShippingFee != 40.0 {
		t.Errorf("expected IntraCity (40.0), got %s (%.2f)", res.Zone, res.ShippingFee)
	}

	// Different cities in Tamil Nadu -> TamilNadu
	res2, _ := calc.Calculate(ctx, &CalculationRequest{
		WeightGrams:      400,
		OriginCity:       "Chennai",
		DestinationCity:  "Coimbatore",
		OriginState:      "Tamil Nadu",
		DestinationState: "Tamil Nadu",
	})
	if res2.Zone != ZoneTamilNadu || res2.ShippingFee != 55.0 {
		t.Errorf("expected TamilNadu (55.0), got %s (%.2f)", res2.Zone, res2.ShippingFee)
	}

	// Different state -> Regional
	res3, _ := calc.Calculate(ctx, &CalculationRequest{
		WeightGrams:      400,
		OriginCity:       "Chennai",
		DestinationCity:  "Bengaluru",
		OriginState:      "Tamil Nadu",
		DestinationState: "Karnataka",
	})
	if res3.Zone != ZoneRegional || res3.ShippingFee != 65.0 {
		t.Errorf("expected Regional (65.0), got %s (%.2f)", res3.Zone, res3.ShippingFee)
	}
}

func TestShippingFeeEngine_PluggableModels(t *testing.T) {
	engine := NewEngine(EngineConfig{
		DefaultModel: ModelWeightBased,
		BaseFlatRate: 49.0,
		Currency:     "INR",
	})
	ctx := context.Background()

	// Default model: weight based
	resWeight, err := engine.Calculate(ctx, &CalculationRequest{
		WeightGrams: 300,
		Zone:        ZoneIntraCity,
	})
	if err != nil || resWeight.ShippingFee != 40.0 {
		t.Fatalf("expected weight based fee 40.0, got %v (err: %v)", resWeight, err)
	}

	// Model override: flat rate
	resFlat, err := engine.Calculate(ctx, &CalculationRequest{
		WeightGrams: 300,
		Zone:        ZoneIntraCity,
		Model:       ModelFlatRate,
	})
	if err != nil || resFlat.ShippingFee != 49.0 {
		t.Fatalf("expected flat rate fee 49.0, got %v (err: %v)", resFlat, err)
	}
}

