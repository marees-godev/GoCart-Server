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
	ProductID string
	VariantID *string
	Quantity  int
}

type ReserveStockInput struct {
	OrderID string
	Items   []ReserveItemInput
}

type ReserveItemInput struct {
	ProductID string
	VariantID *string
	Quantity  int
}
