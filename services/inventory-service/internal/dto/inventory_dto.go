package dto

type CreateInventoryInput struct {
	MerchantID        string
	ProductID         string
	VariantID         *string
	SKU               string
	InitialQuantity   int
	LowStockThreshold int
}

type RestockInventoryInput struct {
	MerchantID  string
	InventoryID *string
	ProductID   *string
	VariantID   *string
	Quantity    int
	ReferenceID *string
	Notes       *string
}

type GetInventoryInput struct {
	InventoryID *string
	ProductID   *string
	VariantID   *string
	MerchantID  *string
}

type UpdateStockInput struct {
	InventoryID *string
	ProductID   *string
	VariantID   *string
	Quantity    int
}

type ReserveStockInput struct {
	OrderID           string
	Items             []ReserveItemInput
	ExpirationMinutes int
}

type ReserveItemInput struct {
	ProductID string
	VariantID *string
	Quantity  int
}

type ReleaseStockInput struct {
	ReservationID string
	OrderID       string
	Reason        string
}

type CommitStockInput struct {
	ReservationID string
	OrderID       string
}
