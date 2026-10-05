package events

import (
	"time"
)

const (
	EventInventoryCreated   = "InventoryCreated"
	EventInventoryRestocked = "InventoryRestocked"
	EventInventoryReserved  = "InventoryReserved"
	EventInventoryReleased  = "InventoryReleased"
	EventInventoryCommitted = "InventoryCommitted"
	EventInventoryLow       = "InventoryLow"
)

type InventoryCreatedEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	SKU               string    `json:"sku"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	LowStockThreshold int       `json:"low_stock_threshold"`
	Timestamp         time.Time `json:"timestamp"`
}

type InventoryRestockedEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	RestockQuantity   int       `json:"restock_quantity"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	Timestamp         time.Time `json:"timestamp"`
}

type InventoryReservedEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	SKU               string    `json:"sku"`
	OrderID           string    `json:"order_id"`
	ReservationID     string    `json:"reservation_id"`
	Quantity          int       `json:"quantity"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	LowStockThreshold int       `json:"low_stock_threshold"`
	Timestamp         time.Time `json:"timestamp"`
}

type InventoryReleasedEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	SKU               string    `json:"sku"`
	OrderID           string    `json:"order_id"`
	ReservationID     string    `json:"reservation_id"`
	Quantity          int       `json:"quantity"`
	Reason            string    `json:"reason,omitempty"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	Timestamp         time.Time `json:"timestamp"`
}

type InventoryCommittedEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	SKU               string    `json:"sku"`
	OrderID           string    `json:"order_id"`
	ReservationID     string    `json:"reservation_id"`
	Quantity          int       `json:"quantity"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	Timestamp         time.Time `json:"timestamp"`
}

type InventoryLowEvent struct {
	EventType         string    `json:"event_type"`
	InventoryID       string    `json:"inventory_id"`
	ProductID         string    `json:"product_id"`
	VariantID         *string   `json:"variant_id,omitempty"`
	SKU               string    `json:"sku"`
	AvailableQuantity int       `json:"available_quantity"`
	ReservedQuantity  int       `json:"reserved_quantity"`
	LowStockThreshold int       `json:"low_stock_threshold"`
	Timestamp         time.Time `json:"timestamp"`
}
