package client

import (
	"context"
	"fmt"
	"time"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
)

type AuthoritativeOrderItem struct {
	ID         string
	ProductID  string
	Quantity   int32
	UnitPrice  float64
	TotalPrice float64
}

type AuthoritativeOrder struct {
	ID          string
	UserID      string
	TotalAmount float64
	Currency    string
	Status      string
	Items       []AuthoritativeOrderItem
}

type OrderClient interface {
	GetOrder(ctx context.Context, orderID string) (*AuthoritativeOrder, error)
}

type grpcOrderClient struct {
	target string
}

func NewOrderClient(target string) OrderClient {
	return &grpcOrderClient{
		target: target,
	}
}

func (c *grpcOrderClient) GetOrder(ctx context.Context, orderID string) (*AuthoritativeOrder, error) {
	if orderID == "" {
		return nil, appErrors.BadRequest("order_id cannot be empty")
	}

	client, conn, err := grpcclient.NewOrderClient(c.target, 5*time.Second)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to connect to order service")
	}
	defer func() { _ = conn.Close() }()

	resp, err := client.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id: orderID,
	})
	if err != nil {
		return nil, appErrors.NotFound(fmt.Sprintf("authoritative order %s not found", orderID))
	}

	if resp == nil || resp.Order == nil {
		return nil, appErrors.NotFound(fmt.Sprintf("authoritative order %s is empty", orderID))
	}

	var items []AuthoritativeOrderItem
	if len(resp.Order.Items) > 0 {
		for _, it := range resp.Order.Items {
			items = append(items, AuthoritativeOrderItem{
				ID:         it.Id,
				ProductID:  it.ProductId,
				Quantity:   it.Quantity,
				UnitPrice:  it.UnitPrice,
				TotalPrice: it.TotalPrice,
			})
		}
	}

	currency := "INR" // Default currency if not present in proto
	if resp.Order.Currency != "" {
		currency = resp.Order.Currency
	}
	return &AuthoritativeOrder{
		ID:          resp.Order.Id,
		UserID:      resp.Order.UserId,
		TotalAmount: resp.Order.TotalAmount,
		Currency:    currency,
		Status:      resp.Order.Status,
		Items:       items,
	}, nil
}
