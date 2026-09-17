package grpc

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/cartpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/orderpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/productpb"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	UserClient    userpb.UserServiceClient
	ProductClient productpb.ProductServiceClient
	CartClient    cartpb.CartServiceClient
	OrderClient   orderpb.OrderServiceClient
	conns         []*grpc.ClientConn
}

func NewClients(cfg *config.Config) (*Clients, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}

	userConn, err := grpc.NewClient(cfg.UserServiceAddr, opts...)
	if err != nil {
		return nil, err
	}

	productConn, err := grpc.NewClient(cfg.ProductServiceAddr, opts...)
	if err != nil {
		userConn.Close()
		return nil, err
	}

	cartConn, err := grpc.NewClient(cfg.CartServiceAddr, opts...)
	if err != nil {
		userConn.Close()
		productConn.Close()
		return nil, err
	}

	orderConn, err := grpc.NewClient(cfg.OrderServiceAddr, opts...)
	if err != nil {
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		return nil, err
	}

	return &Clients{
		UserClient:    userpb.NewUserServiceClient(userConn),
		ProductClient: productpb.NewProductServiceClient(productConn),
		CartClient:    cartpb.NewCartServiceClient(cartConn),
		OrderClient:   orderpb.NewOrderServiceClient(orderConn),
		conns: []*grpc.ClientConn{
			userConn, productConn, cartConn, orderConn,
		},
	}, nil
}

func NewClientsWithServices(
	u userpb.UserServiceClient,
	p productpb.ProductServiceClient,
	c cartpb.CartServiceClient,
	o orderpb.OrderServiceClient,
) *Clients {
	return &Clients{
		UserClient:    u,
		ProductClient: p,
		CartClient:    c,
		OrderClient:   o,
	}
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}
