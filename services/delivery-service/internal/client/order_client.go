package client

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type OrderDetails struct {
	ID              string
	UserID          string
	Status          string
	OrderNumber     string
	ShippingAddress model.ShippingAddress
}

type OrderClient interface {
	GetOrder(ctx context.Context, orderID string) (*OrderDetails, error)
}

type grpcOrderClient struct {
	target string
}

func NewOrderClient(target string) OrderClient {
	return &grpcOrderClient{
		target: target,
	}
}

func (c *grpcOrderClient) GetOrder(ctx context.Context, orderID string) (*OrderDetails, error) {
	cleanID := strings.TrimSpace(orderID)
	if cleanID == "" {
		return nil, appErrors.BadRequest("order_id cannot be empty")
	}

	client, conn, err := grpcclient.NewOrderClient(c.target, 5*time.Second)
	if err != nil {
		return nil, appErrors.Internal(err, "failed to connect to order service")
	}
	defer func() { _ = conn.Close() }()

	resp, err := client.GetOrder(ctx, &orderpb.GetOrderRequest{
		Id: cleanID,
	})
	if err != nil {
		return nil, appErrors.NotFound(fmt.Sprintf("order %s not found in order service", cleanID))
	}

	if resp == nil || resp.Order == nil {
		return nil, appErrors.NotFound(fmt.Sprintf("order %s is empty in order service", cleanID))
	}

	ord := resp.Order
	var addr model.ShippingAddress
	rawAddr := strings.TrimSpace(ord.ShippingAddress)
	if rawAddr != "" {
		addr = model.ParseShippingAddress(rawAddr)
		if addr.ContactNumber == "" {
			var rawMap map[string]interface{}
			if err := json.Unmarshal([]byte(rawAddr), &rawMap); err == nil {
				for _, k := range []string{"contact_number", "contactNumber", "phone_number", "phoneNumber", "phone"} {
					if v, ok := rawMap[k].(string); ok && strings.TrimSpace(v) != "" {
						addr.ContactNumber = strings.TrimSpace(v)
						break
					}
				}
			}
		}
	}

	return &OrderDetails{
		ID:              ord.Id,
		UserID:          ord.UserId,
		Status:          ord.Status,
		OrderNumber:     ord.OrderNumber,
		ShippingAddress: addr,
	}, nil
}
