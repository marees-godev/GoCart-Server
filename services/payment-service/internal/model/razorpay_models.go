package model

// RazorpayWebhookEvent represents the top-level Razorpay webhook event envelope.
type RazorpayWebhookEvent struct {
	Entity    string                 `json:"entity"`
	AccountID string                 `json:"account_id"`
	Event     string                 `json:"event"`
	Contains  []string               `json:"contains"`
	Payload   RazorpayWebhookPayload `json:"payload"`
	CreatedAt int64                  `json:"created_at"`
}

// RazorpayWebhookPayload holds event payload containers for payment, order, and refund entities.
type RazorpayWebhookPayload struct {
	Payment RazorpayPaymentContainer `json:"payment"`
	Order   RazorpayOrderContainer   `json:"order"`
	Refund  RazorpayRefundContainer  `json:"refund"`
}

type RazorpayPaymentContainer struct {
	Entity RazorpayPaymentEntity `json:"entity"`
}

type RazorpayOrderContainer struct {
	Entity RazorpayOrderEntity `json:"entity"`
}

type RazorpayRefundContainer struct {
	Entity RazorpayRefundEntity `json:"entity"`
}

type RazorpayRefundEntity struct {
	ID        string            `json:"id"`
	Entity    string            `json:"entity"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	PaymentID string            `json:"payment_id"`
	Notes     map[string]string `json:"notes"`
	Status    string            `json:"status"`
	CreatedAt int64             `json:"created_at"`
}

type RazorpayPaymentEntity struct {
	ID               string            `json:"id"`
	Entity           string            `json:"entity"`
	Amount           int64             `json:"amount"`
	Currency         string            `json:"currency"`
	Status           string            `json:"status"`
	OrderID          string            `json:"order_id"`
	InvoiceID        string            `json:"invoice_id"`
	Method           string            `json:"method"`
	Captured         bool              `json:"captured"`
	Description      string            `json:"description"`
	Notes            map[string]string `json:"notes"`
	ErrorCode        string            `json:"error_code"`
	ErrorDescription string            `json:"error_description"`
	ErrorSource      string            `json:"error_source"`
	ErrorStep        string            `json:"error_step"`
	ErrorReason      string            `json:"error_reason"`
	CreatedAt        int64             `json:"created_at"`
}

type RazorpayOrderEntity struct {
	ID        string            `json:"id"`
	Entity    string            `json:"entity"`
	Amount    int64             `json:"amount"`
	Currency  string            `json:"currency"`
	Receipt   string            `json:"receipt"`
	Status    string            `json:"status"`
	Notes     map[string]string `json:"notes"`
	CreatedAt int64             `json:"created_at"`
}

type RazorpayOrderRequest struct {
	Amount   int64             `json:"amount"` // Amount in smallest currency unit (paise / cents)
	Currency string            `json:"currency"`
	Receipt  string            `json:"receipt"`
	Notes    map[string]string `json:"notes,omitempty"`
}

type RazorpayOrderResponse struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	Receipt   string `json:"receipt"`
	Status    string `json:"status"`
	CreatedAt int64  `json:"created_at"`
	Error     *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
		Source      string `json:"source"`
		Step        string `json:"step"`
		Reason      string `json:"reason"`
	} `json:"error,omitempty"`
}

type RazorpayRefundRequest struct {
	Amount string            `json:"amount,omitempty"`
	Notes  map[string]string `json:"notes,omitempty"`
}

type RazorpayRefundResponse struct {
	ID        string `json:"id"`
	Entity    string `json:"entity"`
	Amount    int64  `json:"amount"`
	Currency  string `json:"currency"`
	PaymentID string `json:"payment_id"`
	Status    string `json:"status"`
	Error     *struct {
		Code        string `json:"code"`
		Description string `json:"description"`
	} `json:"error,omitempty"`
}
