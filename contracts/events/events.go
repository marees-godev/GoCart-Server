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
	OrderID   string    `json:"order_id"`
	Reason    string    `json:"reason"`
	FailedAt  time.Time `json:"failed_at"`
}

// UserRegisteredEvent represents the payload for auth.user.registered domain event.
type UserRegisteredEvent struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	RegisteredAt time.Time `json:"registered_at"`
}

// MerchantRegisteredEvent represents the payload for auth.merchant.registered domain event.
type MerchantRegisteredEvent struct {
	UserID       string    `json:"user_id"`
	Email        string    `json:"email"`
	Role         string    `json:"role"`
	RegisteredAt time.Time `json:"registered_at"`
}
