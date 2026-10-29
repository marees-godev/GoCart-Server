package client

import (
	"context"

	inventorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
)

type WarehouseInfo struct {
	ID                string
	DeliveryPartnerID string
	Name              string
	Code              string
	City              string
	State             string
	IsActive          bool
}

type InventoryClient interface {
	GetPartnerWarehouses(ctx context.Context, partnerID string) ([]*WarehouseInfo, error)
	GetAvailableWarehouses(ctx context.Context) ([]*WarehouseInfo, error)
}

type grpcInventoryClient struct {
	client inventorypb.InventoryServiceClient
}

func NewGRPCInventoryClient(client inventorypb.InventoryServiceClient) InventoryClient {
	return &grpcInventoryClient{client: client}
}

func (c *grpcInventoryClient) GetPartnerWarehouses(ctx context.Context, partnerID string) ([]*WarehouseInfo, error) {
	if c.client == nil {
		return nil, nil
	}
	resp, err := c.client.ListWarehouses(ctx, &inventorypb.ListWarehousesRequest{
		DeliveryPartnerId: partnerID,
	})
	if err != nil {
		return nil, err
	}
	return mapProtoWarehouses(resp.GetWarehouses()), nil
}

func (c *grpcInventoryClient) GetAvailableWarehouses(ctx context.Context) ([]*WarehouseInfo, error) {
	if c.client == nil {
		return nil, nil
	}
	resp, err := c.client.ListWarehouses(ctx, &inventorypb.ListWarehousesRequest{})
	if err != nil {
		return nil, err
	}
	return mapProtoWarehouses(resp.GetWarehouses()), nil
}

func mapProtoWarehouses(protoList []*inventorypb.Warehouse) []*WarehouseInfo {
	res := make([]*WarehouseInfo, 0, len(protoList))
	for _, w := range protoList {
		if w != nil {
			res = append(res, &WarehouseInfo{
				ID:                w.Id,
				DeliveryPartnerID: w.DeliveryPartnerId,
				Name:              w.Name,
				Code:              w.Code,
				City:              w.City,
				State:             w.State,
				IsActive:          w.IsActive,
			})
		}
	}
	return res
}
