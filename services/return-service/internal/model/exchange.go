package model

import "time"

type ExchangeStatus string

const (
	ExchangeStatusOfferCreated       ExchangeStatus = "OFFER_CREATED"
	ExchangeStatusAppliedAtCheckout ExchangeStatus = "APPLIED_AT_CHECKOUT"
	ExchangeStatusPickupScheduled   ExchangeStatus = "PICKUP_SCHEDULED"
	ExchangeStatusPickedUp          ExchangeStatus = "PICKED_UP"
	ExchangeStatusVerified          ExchangeStatus = "VERIFIED"
	ExchangeStatusRevaluated        ExchangeStatus = "REVALUATED"
	ExchangeStatusCompleted         ExchangeStatus = "COMPLETED"
	ExchangeStatusCancelled         ExchangeStatus = "CANCELLED"
)

func (s ExchangeStatus) IsValid() bool {
	switch s {
	case ExchangeStatusOfferCreated, ExchangeStatusAppliedAtCheckout, ExchangeStatusPickupScheduled,
		ExchangeStatusPickedUp, ExchangeStatusVerified, ExchangeStatusRevaluated,
		ExchangeStatusCompleted, ExchangeStatusCancelled:
		return true
	default:
		return false
	}
}

var AllowedExchangeTransitions = map[ExchangeStatus][]ExchangeStatus{
	ExchangeStatusOfferCreated: {
		ExchangeStatusAppliedAtCheckout,
		ExchangeStatusCancelled,
	},
	ExchangeStatusAppliedAtCheckout: {
		ExchangeStatusPickupScheduled,
		ExchangeStatusPickedUp,
		ExchangeStatusCancelled,
	},
	ExchangeStatusPickupScheduled: {
		ExchangeStatusPickedUp,
		ExchangeStatusCancelled,
	},
	ExchangeStatusPickedUp: {
		ExchangeStatusVerified,
		ExchangeStatusRevaluated,
		ExchangeStatusCancelled,
	},
	ExchangeStatusRevaluated: {
		ExchangeStatusVerified,
		ExchangeStatusCancelled,
	},
	ExchangeStatusVerified: {
		ExchangeStatusCompleted,
		ExchangeStatusCancelled,
	},
	ExchangeStatusCompleted: {},
	ExchangeStatusCancelled: {},
}

func (s ExchangeStatus) CanTransitionTo(target ExchangeStatus) bool {
	if s == target {
		return true
	}
	allowed, ok := AllowedExchangeTransitions[s]
	if !ok {
		return false
	}
	for _, a := range allowed {
		if a == target {
			return true
		}
	}
	return false
}

type ExchangeConditionRating string

const (
	RatingExcellent ExchangeConditionRating = "EXCELLENT"
	RatingGood      ExchangeConditionRating = "GOOD"
	RatingAverage   ExchangeConditionRating = "AVERAGE"
	RatingPoor      ExchangeConditionRating = "POOR"
	RatingDamaged   ExchangeConditionRating = "DAMAGED"
)

func (r ExchangeConditionRating) IsValid() bool {
	switch r {
	case RatingExcellent, RatingGood, RatingAverage, RatingPoor, RatingDamaged:
		return true
	default:
		return false
	}
}

type ExchangeQuestionAnswer struct {
	ID              string    `json:"id" db:"id"`
	ExchangeID      string    `json:"exchange_id" db:"exchange_id"`
	QuestionKey     string    `json:"question_key" db:"question_key"`
	AnswerValue     string    `json:"answer_value" db:"answer_value"`
	DeductionAmount float64   `json:"deduction_amount" db:"deduction_amount"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type ExchangeVerification struct {
	ID               string                  `json:"id" db:"id"`
	ExchangeID       string                  `json:"exchange_id" db:"exchange_id"`
	VerifiedBy       string                  `json:"verified_by" db:"verified_by"`
	ConditionRating  ExchangeConditionRating `json:"condition_rating" db:"condition_rating"`
	IsApproved       bool                    `json:"is_approved" db:"is_approved"`
	ScreenIntact     bool                    `json:"screen_intact" db:"screen_intact"`
	BodyIntact       bool                    `json:"body_intact" db:"body_intact"`
	Functional       bool                    `json:"functional" db:"functional"`
	SerialMatches    bool                    `json:"serial_matches" db:"serial_matches"`
	AdjustedDiscount float64                 `json:"adjusted_discount" db:"adjusted_discount"`
	ProofImages      []string                `json:"proof_images,omitempty"`
	Notes            string                  `json:"notes,omitempty" db:"notes"`
	VerifiedAt       time.Time               `json:"verified_at" db:"verified_at"`
	CreatedAt        time.Time               `json:"created_at" db:"created_at"`
	UpdatedAt        time.Time               `json:"updated_at" db:"updated_at"`
}

type Exchange struct {
	ID                string                    `json:"id" db:"id"`
	OrderID           string                    `json:"order_id" db:"order_id"`
	OrderItemID       string                    `json:"order_item_id" db:"order_item_id"`
	UserID            string                    `json:"user_id" db:"user_id"`
	NewProductID      string                    `json:"new_product_id" db:"new_product_id"`
	OldBrand          string                    `json:"old_brand" db:"old_brand"`
	OldModel          string                    `json:"old_model" db:"old_model"`
	OldVariant        string                    `json:"old_variant,omitempty" db:"old_variant"`
	SerialOrIMEI      string                    `json:"serial_or_imei,omitempty" db:"serial_or_imei"`
	BaseExchangeValue float64                   `json:"base_exchange_value" db:"base_exchange_value"`
	EstimatedDiscount float64                   `json:"estimated_discount" db:"estimated_discount"`
	FinalDiscount     float64                   `json:"final_discount" db:"final_discount"`
	PriceDifference   float64                   `json:"price_difference" db:"price_difference"`
	Status            ExchangeStatus            `json:"status" db:"status"`
	Answers           []*ExchangeQuestionAnswer `json:"answers,omitempty"`
	Verification      *ExchangeVerification     `json:"verification,omitempty"`
	CreatedAt         time.Time                 `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time                 `json:"updated_at" db:"updated_at"`
}
