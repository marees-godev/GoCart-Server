package orderpb

import (
	"context"

	"google.golang.org/grpc"
)

type OrderItem struct {
	Id        string  `json:"id"`
	ProductId string  `json:"product_id"`
	Quantity  int32   `json:"quantity"`
	Price     float64 `json:"price"`
}

type Order struct {
	Id          string       `json:"id"`
	UserId      string       `json:"user_id"`
	Status      string       `json:"status"`
	Items       []*OrderItem `json:"items"`
	TotalAmount float64      `json:"total_amount"`
	CreatedAt   string       `json:"created_at"`
}

type GetOrderRequest struct {
	Id string `json:"id"`
}

type GetOrderResponse struct {
	Order *Order `json:"order"`
}

type CreateOrderRequest struct {
	UserId          string `json:"user_id"`
	CartId          string `json:"cart_id"`
	ShippingAddress string `json:"shipping_address"`
}

type CreateOrderResponse struct {
	Order *Order `json:"order"`
}

type OrderServiceClient interface {
	GetOrder(ctx context.Context, in *GetOrderRequest, opts ...grpc.CallOption) (*GetOrderResponse, error)
	CreateOrder(ctx context.Context, in *CreateOrderRequest, opts ...grpc.CallOption) (*CreateOrderResponse, error)
}

type orderServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewOrderServiceClient(cc grpc.ClientConnInterface) OrderServiceClient {
	return &orderServiceClient{cc: cc}
}

func (c *orderServiceClient) GetOrder(ctx context.Context, in *GetOrderRequest, opts ...grpc.CallOption) (*GetOrderResponse, error) {
	out := new(GetOrderResponse)
	err := c.cc.Invoke(ctx, "/gocart.order.OrderService/GetOrder", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *orderServiceClient) CreateOrder(ctx context.Context, in *CreateOrderRequest, opts ...grpc.CallOption) (*CreateOrderResponse, error) {
	out := new(CreateOrderResponse)
	err := c.cc.Invoke(ctx, "/gocart.order.OrderService/CreateOrder", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}
