package model

import "time"

type ExchangeQuoteStatus string

const (
	ExchangeQuoteStatusQuoted    ExchangeQuoteStatus = "QUOTED"
	ExchangeQuoteStatusLocked    ExchangeQuoteStatus = "LOCKED"
	ExchangeQuoteStatusApplied   ExchangeQuoteStatus = "APPLIED"
	ExchangeQuoteStatusExpired   ExchangeQuoteStatus = "EXPIRED"
	ExchangeQuoteStatusCancelled ExchangeQuoteStatus = "CANCELLED"
)

func (s ExchangeQuoteStatus) IsValid() bool {
	switch s {
	case ExchangeQuoteStatusQuoted, ExchangeQuoteStatusLocked, ExchangeQuoteStatusApplied,
		ExchangeQuoteStatusExpired, ExchangeQuoteStatusCancelled:
		return true
	default:
		return false
	}
}

type ExchangeDeviceCatalog struct {
	ID                string    `json:"id" db:"id"`
	CategoryCode      string    `json:"category_code" db:"category_code"`
	Brand             string    `json:"brand" db:"brand"`
	Model             string    `json:"model" db:"model"`
	Variant           string    `json:"variant" db:"variant"`
	BaseExchangeValue float64   `json:"base_exchange_value" db:"base_exchange_value"`
	IsActive          bool      `json:"is_active" db:"is_active"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
}

type ExchangeQuestionOption struct {
	ID                  string    `json:"id" db:"id"`
	QuestionID          string    `json:"question_id" db:"question_id"`
	OptionKey           string    `json:"option_key" db:"option_key"`
	OptionLabel         string    `json:"option_label" db:"option_label"`
	DeductionAmount     float64   `json:"deduction_amount" db:"deduction_amount"`
	IsFatalDisqualifier bool      `json:"is_fatal_disqualifier" db:"is_fatal_disqualifier"`
	RejectionReason     string    `json:"rejection_reason,omitempty" db:"rejection_reason"`
	DisplayOrder        int       `json:"display_order" db:"display_order"`
	CreatedAt           time.Time `json:"created_at" db:"created_at"`
}

type ExchangeDiagnosticQuestion struct {
	ID           string                    `json:"id" db:"id"`
	CategoryCode string                    `json:"category_code" db:"category_code"`
	QuestionKey  string                    `json:"question_key" db:"question_key"`
	QuestionText string                    `json:"question_text" db:"question_text"`
	DisplayOrder int                       `json:"display_order" db:"display_order"`
	Options      []*ExchangeQuestionOption `json:"options,omitempty"`
	CreatedAt    time.Time                 `json:"created_at" db:"created_at"`
}

type ExchangeQuoteAnswer struct {
	ID              string    `json:"id" db:"id"`
	QuoteID         string    `json:"quote_id" db:"quote_id"`
	QuestionKey     string    `json:"question_key" db:"question_key"`
	OptionKey       string    `json:"option_key" db:"option_key"`
	DeductionAmount float64   `json:"deduction_amount" db:"deduction_amount"`
	IsFatal         bool      `json:"is_fatal" db:"is_fatal"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

type ExchangeQuote struct {
	ID                     string                 `json:"id" db:"id"`
	UserID                 string                 `json:"user_id" db:"user_id"`
	ProductID              string                 `json:"product_id" db:"product_id"`
	CategoryCode           string                 `json:"category_code" db:"category_code"`
	OldBrand               string                 `json:"old_brand" db:"old_brand"`
	OldModel               string                 `json:"old_model" db:"old_model"`
	OldVariant             string                 `json:"old_variant,omitempty" db:"old_variant"`
	SerialOrIMEI           string                 `json:"serial_or_imei,omitempty" db:"serial_or_imei"`
	BaseExchangeValue      float64                `json:"base_exchange_value" db:"base_exchange_value"`
	TotalDeductions        float64                `json:"total_deductions" db:"total_deductions"`
	FinalDiscount          float64                `json:"final_discount" db:"final_discount"`
	IsDisqualified         bool                   `json:"is_disqualified" db:"is_disqualified"`
	DisqualificationReason string                 `json:"disqualification_reason,omitempty" db:"disqualification_reason"`
	Status                 ExchangeQuoteStatus    `json:"status" db:"status"`
	ExpiresAt              time.Time              `json:"expires_at" db:"expires_at"`
	Answers                []*ExchangeQuoteAnswer `json:"answers,omitempty"`
	CreatedAt              time.Time              `json:"created_at" db:"created_at"`
	UpdatedAt              time.Time              `json:"updated_at" db:"updated_at"`
}

func (q *ExchangeQuote) IsExpired(now time.Time) bool {
	return now.After(q.ExpiresAt)
}
