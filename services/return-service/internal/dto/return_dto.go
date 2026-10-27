package dto

type RequestReturnDTO struct {
	OrderID     string   `json:"order_id"`
	OrderItemID string   `json:"order_item_id"`
	UserID      string   `json:"user_id"`
	Reason      string   `json:"reason"`
	ItemIDs     []string `json:"item_ids,omitempty"`
	Type        string   `json:"type,omitempty"`
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

