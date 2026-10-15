package client

import (
	"context"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
)

type CartClient interface {
	GetCart(ctx context.Context, userID string) (*cartpb.Cart, error)
	ClearCart(ctx context.Context, userID string) error
}

type ProductClient interface {
	GetProduct(ctx context.Context, productID string) (*productpb.Product, error)
}

type InventoryClient interface {
	GetStock(ctx context.Context, productID string) (*inventorypb.StockItem, error)
}

type cartClient struct {
	client cartpb.CartServiceClient
}

func NewCartClient(c cartpb.CartServiceClient) CartClient {
	return &cartClient{client: c}
}

func (c *cartClient) GetCart(ctx context.Context, userID string) (*cartpb.Cart, error) {
	resp, err := c.client.GetCart(ctx, &cartpb.GetCartRequest{UserId: userID})
	if err != nil {
		return nil, err
	}
	return resp.GetCart(), nil
}

func (c *cartClient) ClearCart(ctx context.Context, userID string) error {
	_, err := c.client.ClearCart(ctx, &cartpb.ClearCartRequest{UserId: userID})
	return err
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
