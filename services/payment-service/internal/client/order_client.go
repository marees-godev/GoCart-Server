package client

import (
	"context"
	"fmt"
	"time"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
)

type AuthoritativeOrder struct {
	ID          string
	UserID      string
	TotalAmount float64
	Currency    string
	Status      string
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

	currency := "INR" // Default currency if not present in proto
	return &AuthoritativeOrder{
		ID:          resp.Order.Id,
		UserID:      resp.Order.UserId,
		TotalAmount: resp.Order.TotalAmount,
		Currency:    currency,
		Status:      resp.Order.Status,
	}, nil
}
