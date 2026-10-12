package product

type ProductStatus string

const (
	StatusDraft         ProductStatus = "DRAFT"
	StatusPendingReview ProductStatus = "PENDING_REVIEW"
	StatusPublished     ProductStatus = "PUBLISHED"
	StatusUnpublished   ProductStatus = "UNPUBLISHED"
	StatusSuspended     ProductStatus = "SUSPENDED"
	StatusDeleted       ProductStatus = "DELETED"

	// Legacy / Inventory compatibility statuses
	StatusInStock      ProductStatus = "IN_STOCK"
	StatusOutOfStock   ProductStatus = "OUT_OF_STOCK"
	StatusLowStock     ProductStatus = "LOW_STOCK"
	StatusReserved     ProductStatus = "RESERVED"
	StatusDiscontinued ProductStatus = "DISCONTINUED"
)

func (s ProductStatus) String() string {
	return string(s)
}

func (s ProductStatus) IsValid() bool {
	switch s {
	case StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted,
		StatusInStock, StatusOutOfStock, StatusLowStock, StatusReserved, StatusDiscontinued:
		return true
	default:
		return false
	}
}
