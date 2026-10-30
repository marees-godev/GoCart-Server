package events

import "time"

// OrderCreatedEvent represents the payload for OrderCreated domain event.
type OrderCreatedEvent struct {
	OrderID           string             `json:"order_id"`
	ParentOrderID     string             `json:"parent_order_id,omitempty"`
	StoreID           string             `json:"store_id,omitempty"`
	UserID            string             `json:"user_id"`
	Subtotal          float64            `json:"subtotal,omitempty"`
	TaxAmount         float64            `json:"tax_amount,omitempty"`
	ShippingFee       float64            `json:"shipping_fee,omitempty"`
	DiscountAmount    float64            `json:"discount_amount,omitempty"`
	TotalAmount       float64            `json:"total_amount"`
	Currency          string             `json:"currency"`
	PaymentStatus     string             `json:"payment_status,omitempty"`
	Status            string             `json:"status"`
	OrderStatus       string             `json:"order_status,omitempty"`
	ShippingAddressID string             `json:"shipping_address_id,omitempty"`
	BillingAddressID  string             `json:"billing_address_id,omitempty"`
	Items             []OrderItemPayload `json:"items"`
	CreatedAt         time.Time          `json:"created_at"`
}

// OrderItemPayload represents an item entry in OrderCreatedEvent.
type OrderItemPayload struct {
	ProductID   string  `json:"product_id"`
	StoreID     string  `json:"store_id,omitempty"`
	SKU         string  `json:"sku,omitempty"`
	ProductName string  `json:"product_name,omitempty"`
	Quantity    int     `json:"quantity"`
	UnitPrice   float64 `json:"unit_price"`
	Tax         float64 `json:"tax,omitempty"`
	Discount    float64 `json:"discount,omitempty"`
	TotalPrice  float64 `json:"total_price,omitempty"`
}

// PaymentInitiatedEvent represents the payload for PaymentInitiated domain event.
type PaymentInitiatedEvent struct {
	PaymentID     string    `json:"payment_id"`
	OrderID       string    `json:"order_id"`
	UserID        string    `json:"user_id"`
	Amount        float64   `json:"amount"`
	Currency      string    `json:"currency"`
	PaymentMethod string    `json:"payment_method"`
	InitiatedAt   time.Time `json:"initiated_at"`
}

// PaymentPendingEvent represents the payload when a payment enters PENDING status.
type PaymentPendingEvent struct {
	PaymentID            string    `json:"payment_id"`
	OrderID              string    `json:"order_id"`
	UserID               string    `json:"user_id"`
	Amount               float64   `json:"amount"`
	Currency             string    `json:"currency"`
	GatewayTransactionID string    `json:"gateway_transaction_id,omitempty"`
	PendingAt            time.Time `json:"pending_at"`
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

// PaymentCancelledEvent represents the payload for PaymentCancelled domain event.
type PaymentCancelledEvent struct {
	PaymentID   string    `json:"payment_id"`
	OrderID     string    `json:"order_id"`
	UserID      string    `json:"user_id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	CancelledAt time.Time `json:"cancelled_at"`
}

// RefundInitiatedEvent represents the payload for RefundInitiated domain event.
type RefundInitiatedEvent struct {
	RefundID    string    `json:"refund_id"`
	PaymentID   string    `json:"payment_id"`
	OrderID     string    `json:"order_id"`
	Amount      float64   `json:"amount"`
	Currency    string    `json:"currency,omitempty"`
	Reason      string    `json:"reason,omitempty"`
	InitiatedAt time.Time `json:"initiated_at"`
}

// RefundCompletedEvent represents the payload for RefundCompleted domain event.
type RefundCompletedEvent struct {
	RefundID        string    `json:"refund_id"`
	PaymentID       string    `json:"payment_id"`
	OrderID         string    `json:"order_id"`
	Amount          float64   `json:"amount"`
	Currency        string    `json:"currency,omitempty"`
	GatewayRefundID string    `json:"gateway_refund_id,omitempty"`
	IsPartial       bool      `json:"is_partial"`
	PaymentStatus   string    `json:"payment_status,omitempty"`
	CompletedAt     time.Time `json:"completed_at"`
}

// RefundFailedEvent represents the payload for RefundFailed domain event.
type RefundFailedEvent struct {
	RefundID  string    `json:"refund_id"`
	PaymentID string    `json:"payment_id"`
	OrderID   string    `json:"order_id"`
	Amount    float64   `json:"amount"`
	Reason    string    `json:"reason,omitempty"`
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

// ShippingAddressPayload represents a normalized shipping address within event payloads.
type ShippingAddressPayload struct {
	Street        string `json:"street"`
	City          string `json:"city"`
	State         string `json:"state"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
	ContactNumber string `json:"contact_number"`
}

// OrderConfirmedEvent represents the payload for OrderConfirmed domain event.
type OrderConfirmedEvent struct {
	OrderID         string                  `json:"order_id"`
	ShippingAddress *ShippingAddressPayload `json:"shipping_address,omitempty"`
	ConfirmedAt     time.Time               `json:"confirmed_at"`
}

// OrderPaymentFailedEvent represents the payload for OrderPaymentFailed domain event.
type OrderPaymentFailedEvent struct {
	OrderID   string    `json:"order_id"`
	PaymentID string    `json:"payment_id,omitempty"`
	Reason    string    `json:"reason,omitempty"`
	FailedAt  time.Time `json:"failed_at"`
}

// DeliveryCreatedEvent represents the payload for DeliveryCreated domain event.
type DeliveryCreatedEvent struct {
	DeliveryID      string    `json:"delivery_id"`
	OrderID         string    `json:"order_id"`
	DeliveryPartner string    `json:"delivery_partner"`
	TrackingNumber  string    `json:"tracking_number"`
	Status          string    `json:"status"`
	CreatedAt       time.Time `json:"created_at"`
}

// DeliveryStatusUpdatedEvent represents the payload for DeliveryStatusUpdated domain event.
type DeliveryStatusUpdatedEvent struct {
	DeliveryID      string    `json:"delivery_id"`
	OrderID         string    `json:"order_id"`
	PreviousStatus  string    `json:"previous_status"`
	NewStatus       string    `json:"new_status"`
	TrackingNumber  string    `json:"tracking_number"`
	UpdatedAt       time.Time `json:"updated_at"`
}

// DeliveryStatusChangedEvent represents the payload for DeliveryStatusChanged domain event.
type DeliveryStatusChangedEvent struct {
	DeliveryID        string     `json:"delivery_id"`
	OrderID           string     `json:"order_id"`
	PreviousStatus    string     `json:"previous_status"`
	NewStatus         string     `json:"new_status"`
	TrackingNumber    string     `json:"tracking_number"`
	EstimatedDelivery *time.Time `json:"estimated_delivery,omitempty"`
	ActualDelivery    *time.Time `json:"actual_delivery,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

// DeliveryDispatchedEvent represents the payload for DeliveryDispatched domain event.
type DeliveryDispatchedEvent struct {
	DeliveryID      string    `json:"delivery_id"`
	OrderID         string    `json:"order_id"`
	DeliveryPartner string    `json:"delivery_partner"`
	TrackingNumber  string    `json:"tracking_number"`
	DispatchedAt    time.Time `json:"dispatched_at"`
}


// ProductCreatedEvent represents the payload for ProductCreated domain event.
type ProductCreatedEvent struct {
	ProductID   string    `json:"product_id"`
	StoreID     string    `json:"store_id"`
	CategoryID  string    `json:"category_id"`
	SKU         string    `json:"sku"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"created_at"`
}

// ProductUpdatedEvent represents the payload for ProductUpdated domain event.
type ProductUpdatedEvent struct {
	ProductID   string    `json:"product_id"`
	StoreID     string    `json:"store_id"`
	CategoryID  string    `json:"category_id,omitempty"`
	SKU         string    `json:"sku,omitempty"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	Status      string    `json:"status"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// ProductPublishedEvent represents the payload for ProductPublished domain event.
type ProductPublishedEvent struct {
	ProductID   string    `json:"product_id"`
	StoreID     string    `json:"store_id"`
	CategoryID  string    `json:"category_id,omitempty"`
	SKU         string    `json:"sku,omitempty"`
	Name        string    `json:"name"`
	Price       float64   `json:"price"`
	Currency    string    `json:"currency"`
	PublishedAt time.Time `json:"published_at"`
}

// ProductDeletedEvent represents the payload for ProductDeleted domain event.
type ProductDeletedEvent struct {
	ProductID string    `json:"product_id"`
	StoreID   string    `json:"store_id"`
	Status    string    `json:"status"`
	DeletedAt time.Time `json:"deleted_at"`
}

// ReturnRequestedEvent represents the payload for ReturnRequested domain event.
type ReturnRequestedEvent struct {
	ReturnID     string    `json:"return_id"`
	OrderID      string    `json:"order_id"`
	OrderItemID  string    `json:"order_item_id"`
	ProductID    string    `json:"product_id"`
	UserID       string    `json:"user_id"`
	Reason       string    `json:"reason"`
	Status       string    `json:"status"`
	Type         string    `json:"type,omitempty"`
	RefundAmount float64   `json:"refund_amount"`
	RequestedAt  time.Time `json:"requested_at"`
}

// ReturnApprovedEvent represents the payload for ReturnApproved domain event.
type ReturnApprovedEvent struct {
	ReturnID    string    `json:"return_id"`
	OrderID     string    `json:"order_id"`
	OrderItemID string    `json:"order_item_id"`
	UserID      string    `json:"user_id"`
	ApprovedBy  string    `json:"approved_by,omitempty"`
	ApprovedAt  time.Time `json:"approved_at"`
}

// ReturnRejectedEvent represents the payload for ReturnRejected domain event.
type ReturnRejectedEvent struct {
	ReturnID        string    `json:"return_id"`
	OrderID         string    `json:"order_id"`
	OrderItemID     string    `json:"order_item_id"`
	UserID          string    `json:"user_id"`
	RejectedBy      string    `json:"rejected_by,omitempty"`
	RejectionReason string    `json:"rejection_reason"`
	RejectedAt      time.Time `json:"rejected_at"`
}

// ReturnVerifiedEvent represents the payload for ReturnVerified domain event.
type ReturnVerifiedEvent struct {
	ReturnID    string    `json:"return_id"`
	OrderID     string    `json:"order_id"`
	OrderItemID string    `json:"order_item_id"`
	VerifiedBy  string    `json:"verified_by"`
	Condition   string    `json:"condition"`
	IsApproved  bool      `json:"is_approved"`
	ProofImages []string  `json:"proof_images"`
	Notes       string    `json:"notes,omitempty"`
	VerifiedAt  time.Time `json:"verified_at"`
}

// ReturnStatusUpdatedEvent represents the payload for ReturnStatusUpdated domain event.
type ReturnStatusUpdatedEvent struct {
	ReturnID       string    `json:"return_id"`
	OrderID        string    `json:"order_id"`
	OrderItemID    string    `json:"order_item_id"`
	PreviousStatus string    `json:"previous_status"`
	NewStatus      string    `json:"new_status"`
	UpdatedAt      time.Time `json:"updated_at"`
}


// PartnerRegisteredEvent represents the payload for PartnerRegistered domain event.
type PartnerRegisteredEvent struct {
	PartnerID     string `json:"partner_id"`
	CompanyName   string `json:"company_name"`
	Email         string `json:"email"`
	ContactNumber string `json:"contact_number"`
}

