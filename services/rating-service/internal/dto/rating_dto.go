package dto

type CreateRatingInput struct {
	UserID      string `json:"user_id"`
	ProductID   string `json:"product_id"`
	OrderID     string `json:"order_id"`
	OrderItemID string `json:"order_item_id"`
	Rating      int    `json:"rating"`
	Review      string `json:"review"`
}

type UpdateRatingInput struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
	Rating int    `json:"rating"`
	Review string `json:"review"`
}

type DeleteRatingInput struct {
	ID     string `json:"id"`
	UserID string `json:"user_id"`
}
