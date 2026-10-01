package client

import (
	"context"

	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
)

type ProductClient interface {
	GetProduct(ctx context.Context, productID string) (*productpb.Product, error)
}

type InventoryClient interface {
	GetStock(ctx context.Context, productID string) (*inventorypb.StockItem, error)
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

type inventoryClient struct {
	client inventorypb.InventoryServiceClient
}

func NewInventoryClient(c inventorypb.InventoryServiceClient) InventoryClient {
	return &inventoryClient{client: c}
}

func (i *inventoryClient) GetStock(ctx context.Context, productID string) (*inventorypb.StockItem, error) {
	resp, err := i.client.GetStock(ctx, &inventorypb.GetStockRequest{ProductId: productID})
	if err != nil {
		return nil, err
	}
	return resp.GetStock(), nil
}
