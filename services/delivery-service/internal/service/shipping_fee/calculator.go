package shipping_fee

import (
	"context"
	"fmt"
	"math"
	"strings"
)

type WeightBracketCalculator struct {
	Currency string
}

func NewWeightBracketCalculator(currency ...string) *WeightBracketCalculator {
	curr := DefaultCurrency
	if len(currency) > 0 && strings.TrimSpace(currency[0]) != "" {
		curr = strings.TrimSpace(currency[0])
	}
	return &WeightBracketCalculator{Currency: curr}
}

func (c *WeightBracketCalculator) Calculate(ctx context.Context, req *CalculationRequest) (*CalculationResult, error) {
	zone := ResolveZone(req)

	var itemsWeight float64
	for _, item := range req.Items {
		qty := item.Quantity
		if qty <= 0 {
			qty = 1
		}
		itemsWeight += item.WeightGrams * float64(qty)
	}

	totalWeight := req.WeightGrams
	if itemsWeight > totalWeight {
		totalWeight = itemsWeight
	}
	if totalWeight < 0 {
		totalWeight = 0
	}

	var fee float64
	var slabDesc string

	switch {
	case totalWeight <= 500:
		slabDesc = "0–500 g"
		switch zone {
		case ZoneIntraCity:
			fee = 40.0
		case ZoneTamilNadu:
			fee = 55.0
		default:
			fee = 65.0
		}
	case totalWeight <= 1000:
		slabDesc = "501 g–1 kg"
		switch zone {
		case ZoneIntraCity:
			fee = 50.0
		case ZoneTamilNadu:
			fee = 70.0
		default:
			fee = 90.0
		}
	case totalWeight <= 2000:
		slabDesc = "1–2 kg"
		switch zone {
		case ZoneIntraCity:
			fee = 65.0
		case ZoneTamilNadu:
			fee = 90.0
		default:
			fee = 120.0
		}
	case totalWeight <= 3000:
		slabDesc = "2–3 kg"
		switch zone {
		case ZoneIntraCity:
			fee = 85.0
		case ZoneTamilNadu:
			fee = 120.0
		default:
			fee = 160.0
		}
	case totalWeight <= 5000:
		slabDesc = "3–5 kg"
		switch zone {
		case ZoneIntraCity:
			fee = 120.0
		case ZoneTamilNadu:
			fee = 170.0
		default:
			fee = 230.0
		}
	default:
		// Above 5 kg: Base fee for 5kg + additional per kg (rounded up)
		additionalKg := int(math.Ceil((totalWeight - 5000.0) / 1000.0))
		var baseFee float64
		var ratePerKg float64
		switch zone {
		case ZoneIntraCity:
			baseFee = 120.0
			ratePerKg = 25.0
		case ZoneTamilNadu:
			baseFee = 170.0
			ratePerKg = 35.0
		default:
			baseFee = 230.0
			ratePerKg = 50.0
		}
		additionalFee := float64(additionalKg) * ratePerKg
		fee = baseFee + additionalFee
		slabDesc = fmt.Sprintf("3–5 kg base (₹%.2f) + %d additional kg @ ₹%.2f/kg (₹%.2f)", baseFee, additionalKg, ratePerKg, additionalFee)
	}

	breakdown := fmt.Sprintf("Zone: %s, Weight: %.2fg, Slab: %s, Total: ₹%.2f %s", zone, totalWeight, slabDesc, fee, c.Currency)

	return &CalculationResult{
		ShippingFee:      fee,
		Currency:         c.Currency,
		Zone:             zone,
		TotalWeightGrams: totalWeight,
		CalculationModel: ModelWeightBased,
		BreakdownDetails: breakdown,
	}, nil
}

type FlatRateCalculator struct {
	BaseFee  float64
	Currency string
}

func NewFlatRateCalculator(baseFee float64, currency ...string) *FlatRateCalculator {
	curr := DefaultCurrency
	if len(currency) > 0 && strings.TrimSpace(currency[0]) != "" {
		curr = strings.TrimSpace(currency[0])
	}
	if baseFee <= 0 {
		baseFee = 50.0
	}
	return &FlatRateCalculator{
		BaseFee:  baseFee,
		Currency: curr,
	}
}

func (c *FlatRateCalculator) Calculate(ctx context.Context, req *CalculationRequest) (*CalculationResult, error) {
	zone := ResolveZone(req)
	var itemsWeight float64
	for _, item := range req.Items {
		qty := item.Quantity
		if qty <= 0 {
			qty = 1
		}
		itemsWeight += item.WeightGrams * float64(qty)
	}
	totalWeight := req.WeightGrams
	if itemsWeight > totalWeight {
		totalWeight = itemsWeight
	}

	breakdown := fmt.Sprintf("Flat rate fee: ₹%.2f %s across zone %s", c.BaseFee, c.Currency, zone)
	return &CalculationResult{
		ShippingFee:      c.BaseFee,
		Currency:         c.Currency,
		Zone:             zone,
		TotalWeightGrams: totalWeight,
		CalculationModel: ModelFlatRate,
		BreakdownDetails: breakdown,
	}, nil
}

type Engine struct {
	defaultModel string
	calculators  map[string]Calculator
}

type EngineConfig struct {
	DefaultModel string
	BaseFlatRate float64
	Currency     string
}

func NewEngine(cfg EngineConfig) *Engine {
	model := strings.ToUpper(strings.TrimSpace(cfg.DefaultModel))
	if model == "" {
		model = ModelWeightBased
	}

	curr := cfg.Currency
	if curr == "" {
		curr = DefaultCurrency
	}

	weightCalc := NewWeightBracketCalculator(curr)
	flatCalc := NewFlatRateCalculator(cfg.BaseFlatRate, curr)

	calcs := map[string]Calculator{
		ModelWeightBased: weightCalc,
		ModelFlatRate:    flatCalc,
		ModelZoneTiered:  weightCalc, // ZoneTiered uses weight brackets across zone tiers
	}

	return &Engine{
		defaultModel: model,
		calculators:  calcs,
	}
}

func (e *Engine) RegisterCalculator(model string, calc Calculator) {
	if calc != nil && model != "" {
		e.calculators[strings.ToUpper(strings.TrimSpace(model))] = calc
	}
}

func (e *Engine) Calculate(ctx context.Context, req *CalculationRequest) (*CalculationResult, error) {
	modelName := e.defaultModel
	if req != nil && strings.TrimSpace(req.Model) != "" {
		modelName = strings.ToUpper(strings.TrimSpace(req.Model))
	}

	calc, exists := e.calculators[modelName]
	if !exists {
		calc = e.calculators[ModelWeightBased]
	}

	return calc.Calculate(ctx, req)
}

