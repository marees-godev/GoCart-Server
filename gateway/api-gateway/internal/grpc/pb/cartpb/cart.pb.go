package cartpb

import (
	"context"

	"google.golang.org/grpc"
)

type CartItem struct {
	Id        string  `json:"id"`
	ProductId string  `json:"product_id"`
	Quantity  int32   `json:"quantity"`
	UnitPrice float64 `json:"unit_price"`
}

type Cart struct {
	Id          string      `json:"id"`
	UserId      string      `json:"user_id"`
	Items       []*CartItem `json:"items"`
	TotalAmount float64     `json:"total_amount"`
}

type GetCartRequest struct {
	UserId string `json:"user_id"`
}

type GetCartResponse struct {
	Cart *Cart `json:"cart"`
}

type AddToCartRequest struct {
	UserId    string `json:"user_id"`
	ProductId string `json:"product_id"`
	Quantity  int32  `json:"quantity"`
}

type AddToCartResponse struct {
	Cart *Cart `json:"cart"`
}

type CartServiceClient interface {
	GetCart(ctx context.Context, in *GetCartRequest, opts ...grpc.CallOption) (*GetCartResponse, error)
	AddToCart(ctx context.Context, in *AddToCartRequest, opts ...grpc.CallOption) (*AddToCartResponse, error)
}

type cartServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewCartServiceClient(cc grpc.ClientConnInterface) CartServiceClient {
	return &cartServiceClient{cc: cc}
}

func (c *cartServiceClient) GetCart(ctx context.Context, in *GetCartRequest, opts ...grpc.CallOption) (*GetCartResponse, error) {
	out := new(GetCartResponse)
	err := c.cc.Invoke(ctx, "/gocart.cart.CartService/GetCart", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *cartServiceClient) AddToCart(ctx context.Context, in *AddToCartRequest, opts ...grpc.CallOption) (*AddToCartResponse, error) {
	out := new(AddToCartResponse)
	err := c.cc.Invoke(ctx, "/gocart.cart.CartService/AddToCart", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
