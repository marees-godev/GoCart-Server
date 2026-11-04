package dto

type RequestReturnDTO struct {
	OrderID     string `json:"order_id"`
	OrderItemID string `json:"order_item_id"`
	ItemID      string `json:"item_id,omitempty"`
	UserID      string `json:"user_id"`
	Reason      string `json:"reason"`
	Type        string `json:"type,omitempty"`
}

type GetReturnDTO struct {
	ID string `json:"id"`
}

type UpdateReturnStatusDTO struct {
	ID     string `json:"id"`
	Status string `json:"status"`
}

type VerifyProductDTO struct {
	ReturnID    string   `json:"return_id"`
	VerifiedBy  string   `json:"verified_by"`
	Condition   string   `json:"condition"`
	IsApproved  bool     `json:"is_approved"`
	ProofImages []string `json:"proof_images,omitempty"`
	Notes       string   `json:"notes,omitempty"`
}

type ListReturnsDTO struct {
	Limit  int    `json:"limit"`
	Offset int    `json:"offset"`
	Status string `json:"status"`
	UserID string `json:"user_id"`
	Type   string `json:"type"`
}

type ExchangeAnswerDTO struct {
	QuestionKey     string  `json:"question_key"`
	AnswerValue     string  `json:"answer_value"`
	DeductionAmount float64 `json:"deduction_amount"`
}

type CreateExchangeDTO struct {
	ProductID               string               `json:"product_id"`
	ExchangeDeviceCatalogID string               `json:"exchange_device_catalog_id"`
	UserID                  string               `json:"user_id"`
	OldBrand                string               `json:"old_brand,omitempty"`
	OldModel                string               `json:"old_model,omitempty"`
	OldVariant              string               `json:"old_variant,omitempty"`
	SerialOrIMEI            string               `json:"serial_or_imei,omitempty"`
	Answers                 []*ExchangeAnswerDTO `json:"answers,omitempty"`
}

type VerifyExchangeProductDTO struct {
	ExchangeID       string   `json:"exchange_id"`
	VerifiedBy       string   `json:"verified_by"`
	ConditionRating  string   `json:"condition_rating"`
	IsApproved       bool     `json:"is_approved"`
	ScreenIntact     bool     `json:"screen_intact"`
	BodyIntact       bool     `json:"body_intact"`
	Functional       bool     `json:"functional"`
	SerialMatches    bool     `json:"serial_matches"`
	AdjustedDiscount *float64 `json:"adjusted_discount,omitempty"`
	ProofImages      []string `json:"proof_images,omitempty"`
	Notes            string   `json:"notes,omitempty"`
}

type GetExchangeDTO struct {
	ID      string `json:"id,omitempty"`
	OrderID string `json:"order_id,omitempty"`
}

type GetExchangeQuestionsDTO struct {
	CategoryCode string `json:"category_code"`
	ProductID    string `json:"product_id,omitempty"`
}

type ExchangeUserAnswerDTO struct {
	QuestionKey string `json:"question_key"`
	OptionKey   string `json:"option_key"`
}

type ExchangeDeductionDetailDTO struct {
	QuestionKey     string  `json:"question_key"`
	OptionKey       string  `json:"option_key"`
	DeductionAmount float64 `json:"deduction_amount"`
	IsFatal         bool    `json:"is_fatal"`
	RejectionReason string  `json:"rejection_reason,omitempty"`
}

type CalculateExchangeValuationDTO struct {
	NewProductID string                   `json:"new_product_id"`
	CategoryCode string                   `json:"category_code"`
	OldBrand     string                   `json:"old_brand"`
	OldModel     string                   `json:"old_model"`
	OldVariant   string                   `json:"old_variant,omitempty"`
	Answers      []*ExchangeUserAnswerDTO `json:"answers,omitempty"`
}

type ExchangeValuationResultDTO struct {
	IsEligible        bool                          `json:"is_eligible"`
	RejectionReason   string                        `json:"rejection_reason,omitempty"`
	BaseExchangeValue float64                       `json:"base_exchange_value"`
	TotalDeductions   float64                       `json:"total_deductions"`
	FinalDiscount     float64                       `json:"final_discount"`
	Deductions        []*ExchangeDeductionDetailDTO `json:"deductions,omitempty"`
}

type LockExchangeQuoteDTO struct {
	UserID       string                   `json:"user_id"`
	NewProductID string                   `json:"new_product_id"`
	CategoryCode string                   `json:"category_code"`
	OldBrand     string                   `json:"old_brand"`
	OldModel     string                   `json:"old_model"`
	OldVariant   string                   `json:"old_variant,omitempty"`
	SerialOrIMEI string                   `json:"serial_or_imei,omitempty"`
	Answers      []*ExchangeUserAnswerDTO `json:"answers,omitempty"`
}

type ApplyExchangeToOrderDTO struct {
	QuoteID     string `json:"quote_id"`
	OrderID     string `json:"order_id"`
	OrderItemID string `json:"order_item_id"`
	UserID      string `json:"user_id"`
}

type ResolveExchangeMismatchDTO struct {
	ExchangeID    string `json:"exchange_id"`
	Action        string `json:"action"` // "ACCEPT_REVISED" or "DECLINE_AND_CANCEL"
	PaymentMethod string `json:"payment_method,omitempty"`
	Notes         string `json:"notes,omitempty"`
}
