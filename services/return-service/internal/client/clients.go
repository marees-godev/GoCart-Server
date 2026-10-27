package client

import (
	"context"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
)

type OrderClient interface {
	GetOrder(ctx context.Context, orderID, userID string) (*orderpb.Order, error)
}

type ProductClient interface {
	GetProduct(ctx context.Context, productID string) (*productpb.Product, error)
}

type orderClient struct {
	client orderpb.OrderServiceClient
}

func NewOrderClient(c orderpb.OrderServiceClient) OrderClient {
	return &orderClient{client: c}
}

func (o *orderClient) GetOrder(ctx context.Context, orderID, userID string) (*orderpb.Order, error) {
	resp, err := o.client.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id:     orderID,
		UserId: userID,
	})
	if err != nil {
		return nil, err
	}
	return resp.GetOrder(), nil
}

type productClient struct {
	client productpb.ProductServiceClient
}

func NewProductClient(c productpb.ProductServiceClient) ProductClient {
	return &productClient{client: c}
}

func (p *productClient) GetProduct(ctx context.Context, productID string) (*productpb.Product, error) {
	resp, err := p.client.GetProduct(ctx, &productpb.GetProductRequest{Id: productID})
	if err != nil {
		return nil, err
	}
	return resp.GetProduct(), nil
}
