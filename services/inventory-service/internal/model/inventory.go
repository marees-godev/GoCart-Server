package model

import "time"

type TransactionType string

const (
	TransactionTypeRestock    TransactionType = "RESTOCK"
	TransactionTypeReserve    TransactionType = "RESERVE"
	TransactionTypeRelease    TransactionType = "RELEASE"
	TransactionTypeAdjustment TransactionType = "ADJUSTMENT"
	TransactionTypeSale       TransactionType = "SALE"
	TransactionTypeReturn     TransactionType = "RETURN"
	TransactionTypeCommit     TransactionType = "COMMIT"
)

type ReservationStatus string

const (
	ReservationStatusReserved  ReservationStatus = "RESERVED"
	ReservationStatusConfirmed ReservationStatus = "CONFIRMED"
	ReservationStatusReleased  ReservationStatus = "RELEASED"
	ReservationStatusExpired   ReservationStatus = "EXPIRED"
)

type Inventory struct {
	ID                string    `json:"id" db:"id"`
	ProductID         string    `json:"product_id" db:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty" db:"variant_id"`
	SKU               string    `json:"sku" db:"sku"`
	AvailableQuantity int       `json:"available_quantity" db:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity" db:"reserved_quantity"`
	LowStockThreshold int       `json:"low_stock_threshold" db:"low_stock_threshold"`
	CreatedAt         time.Time `json:"created_at" db:"created_at"`
	UpdatedAt         time.Time `json:"updated_at" db:"updated_at"`
}

type InventoryTransaction struct {
	ID          string          `json:"id" db:"id"`
	InventoryID string          `json:"inventory_id" db:"inventory_id"`
	Type        TransactionType `json:"type" db:"type"`
	Quantity    int             `json:"quantity" db:"quantity"`
	ReferenceID *string         `json:"reference_id,omitempty" db:"reference_id"`
	Notes       *string         `json:"notes,omitempty" db:"notes"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
}

type InventoryReservation struct {
	ID        string            `json:"id" db:"id"`
	OrderID   string            `json:"order_id" db:"order_id"`
	ProductID string            `json:"product_id" db:"product_id"`
	VariantID *string           `json:"variant_id,omitempty" db:"variant_id"`
	Quantity  int               `json:"quantity" db:"quantity"`
	Status    ReservationStatus `json:"status" db:"status"`
	ExpiresAt time.Time         `json:"expires_at" db:"expires_at"`
	CreatedAt time.Time         `json:"created_at" db:"created_at"`
	UpdatedAt time.Time         `json:"updated_at" db:"updated_at"`
}

type ReserveItem struct {
	ProductID string
	VariantID *string
	Quantity  int
}
