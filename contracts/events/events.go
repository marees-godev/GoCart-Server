package events

import "time"

// OrderCreatedEvent represents the payload for OrderCreated domain event.
type OrderCreatedEvent struct {
	OrderID     string             `json:"order_id"`
	UserID      string             `json:"user_id"`
	TotalAmount float64            `json:"total_amount"`
	Currency    string             `json:"currency"`
	Status      string             `json:"status"`
	Items       []OrderItemPayload `json:"items"`
	CreatedAt   time.Time          `json:"created_at"`
}

// OrderItemPayload represents an item entry in OrderCreatedEvent.
type OrderItemPayload struct {
	ProductID string  `json:"product_id"`
	Quantity  int     `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

// PaymentSuccessfulEvent represents the payload for PaymentSuccessful domain event.
type PaymentSuccessfulEvent struct {
	PaymentID     string    `json:"payment_id"`
	OrderID       string    `json:"order_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	PaidAt        time.Time `json:"paid_at"`
}

// PaymentFailedEvent represents the payload for PaymentFailed domain event.
type PaymentFailedEvent struct {
	PaymentID string    `json:"payment_id"`
	OrderID   string    `json:"order_id"`
	UserID    string    `json:"user_id"`
	Amount    float64   `json:"amount"`
	Reason    string    `json:"reason"`
	FailedAt  time.Time `json:"failed_at"`
}

// InventoryReservedEvent represents the payload for InventoryReserved domain event.
type InventoryReservedEvent struct {
	ReservationID string                `json:"reservation_id"`
	OrderID       string                `json:"order_id"`
	Items         []ReservedItemPayload `json:"items"`
	ReservedAt    time.Time             `json:"reserved_at"`
}

// ReservedItemPayload represents an item reserved in InventoryReservedEvent.
type ReservedItemPayload struct {
	ProductID string `json:"product_id"`
	Quantity  int    `json:"quantity"`
}

// InventoryReservationFailedEvent represents the payload when inventory reservation fails.
type InventoryReservationFailedEvent struct {
	OrderID  string    `json:"order_id"`
	Reason   string    `json:"reason"`
	FailedAt time.Time `json:"failed_at"`
}

// UserRegisteredEvent represents the payload for auth.user.registered domain event.
type UserRegisteredEvent struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	BusinessName string    `json:"business_name,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
}

// MerchantRegisteredEvent represents the payload for merchant registration domain events.
type MerchantRegisteredEvent struct {
	MerchantID   string    `json:"merchant_id,omitempty"`
	Email        string    `json:"email"`
	Role         string    `json:"role,omitempty"`
	BusinessName string    `json:"business_name,omitempty"`
	FirstName    string    `json:"first_name,omitempty"`
	LastName     string    `json:"last_name,omitempty"`
	Phone        string    `json:"phone,omitempty"`
	CreatedAt    time.Time `json:"created_at,omitempty"`
	RegisteredAt time.Time `json:"registered_at,omitempty"`
}

// MerchantActivatedEvent represents the payload for MerchantActivated domain event.
type MerchantActivatedEvent struct {
	MerchantID     string    `json:"merchant_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	ActivatedBy    string    `json:"activated_by"`
	Reason         string    `json:"reason,omitempty"`
	ActivatedAt    time.Time `json:"activated_at"`
}

// MerchantSuspendedEvent represents the payload for MerchantSuspended domain event.
type MerchantSuspendedEvent struct {
	MerchantID     string    `json:"merchant_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	SuspendedBy    string    `json:"suspended_by"`
	Reason         string    `json:"reason"`
	SuspendedAt    time.Time `json:"suspended_at"`
}

// StoreCreatedEvent represents the payload for StoreCreated domain event.
type StoreCreatedEvent struct {
	StoreID       string    `json:"store_id"`
	MerchantID    string    `json:"merchant_id"`
	Name          string    `json:"name"`
	Slug          string    `json:"slug"`
	BusinessEmail string    `json:"business_email,omitempty"`
	BusinessPhone string    `json:"business_phone,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

// StoreSubmittedEvent represents the payload for StoreSubmitted domain event.
type StoreSubmittedEvent struct {
	StoreID     string    `json:"store_id"`
	MerchantID  string    `json:"merchant_id"`
	SubmittedAt time.Time `json:"submitted_at"`
}

// StoreApprovedEvent represents the payload for StoreApproved domain event.
type StoreApprovedEvent struct {
	StoreID    string    `json:"store_id"`
	MerchantID string    `json:"merchant_id"`
	AdminID    string    `json:"admin_id,omitempty"`
	ApprovedAt time.Time `json:"approved_at"`
}

// StoreRejectedEvent represents the payload for StoreRejected domain event.
type StoreRejectedEvent struct {
	StoreID    string    `json:"store_id"`
	MerchantID string    `json:"merchant_id"`
	AdminID    string    `json:"admin_id,omitempty"`
	Reason     string    `json:"reason"`
	RejectedAt time.Time `json:"rejected_at"`
}

// StoreSuspendedEvent represents the payload for StoreSuspended domain event.
type StoreSuspendedEvent struct {
	StoreID     string    `json:"store_id"`
	MerchantID  string    `json:"merchant_id"`
	AdminID     string    `json:"admin_id,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	SuspendedAt time.Time `json:"suspended_at"`
}

// StoreUnsuspendedEvent represents the payload when a store suspension is lifted by an admin.
type StoreUnsuspendedEvent struct {
	StoreID       string    `json:"store_id"`
	MerchantID    string    `json:"merchant_id"`
	AdminID       string    `json:"admin_id,omitempty"`
	Reason        string    `json:"reason,omitempty"`
	UnsuspendedAt time.Time `json:"unsuspended_at"`
}

// StoreAppealedEvent represents the payload when a merchant submits an appeal for a suspended/rejected store.
type StoreAppealedEvent struct {
	AppealID   string    `json:"appeal_id"`
	StoreID    string    `json:"store_id"`
	MerchantID string    `json:"merchant_id"`
	Reason     string    `json:"reason"`
	AppealedAt time.Time `json:"appealed_at"`
}

// OrderCancelledEvent represents the payload for OrderCancelled domain event.
type OrderCancelledEvent struct {
	OrderID     string    `json:"order_id"`
	Reason      string    `json:"reason,omitempty"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// OrderConfirmedEvent represents the payload for OrderConfirmed domain event.
type OrderConfirmedEvent struct {
	OrderID     string    `json:"order_id"`
	ConfirmedAt time.Time `json:"confirmed_at"`
}
