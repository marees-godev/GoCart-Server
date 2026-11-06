package shipping_fee

import "context"

const (
	ZoneIntraCity = "INTRA_CITY"
	ZoneTamilNadu = "TAMIL_NADU"
	ZoneRegional  = "REGIONAL"

	ModelWeightBased = "WEIGHT_BASED"
	ModelFlatRate    = "FLAT_RATE"
	ModelZoneTiered  = "ZONE_TIERED"

	DefaultCurrency = "INR"
)

type CalculationItem struct {
	ProductID   string  `json:"product_id"`
	Quantity    int     `json:"quantity"`
	WeightGrams float64 `json:"weight_grams"`
}

type CalculationRequest struct {
	WeightGrams           float64           `json:"weight_grams"`
	Zone                  string            `json:"zone"`
	OriginCity            string            `json:"origin_city"`
	OriginState           string            `json:"origin_state"`
	DestinationCity       string            `json:"destination_city"`
	DestinationState      string            `json:"destination_state"`
	DestinationPostalCode string            `json:"destination_postal_code"`
	ShippingAddress       string            `json:"shipping_address"`
	Items                 []CalculationItem `json:"items"`
	Model                 string            `json:"model"`
	PartnerID             string            `json:"partner_id"`
}
 
type CalculationResult struct {
	ShippingFee       float64 `json:"shipping_fee"`
	Currency          string  `json:"currency"`
	Zone              string  `json:"zone"`
	TotalWeightGrams  float64 `json:"total_weight_grams"`
	CalculationModel  string  `json:"calculation_model"`
	BreakdownDetails  string  `json:"breakdown_details"`
}

type Calculator interface {
	Calculate(ctx context.Context, req *CalculationRequest) (*CalculationResult, error)
}
