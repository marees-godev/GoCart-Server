package grpc

import (
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	UserClient userpb.UserServiceClient
	conns      []*grpc.ClientConn
}

func NewClients(cfg *config.Config, extraOpts ...grpc.DialOption) (*Clients, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcclient.UnaryClientInterceptor(cfg.GRPC.DefaultTimeout)),
	}
	opts = append(opts, extraOpts...)

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
		UserClient: userpb.NewUserServiceClient(userConn),
		conns: []*grpc.ClientConn{
			userConn, productConn, cartConn, orderConn,
		},
	}, nil
}

func NewClientsWithServices(
	u userpb.UserServiceClient,
) *Clients {
	return &Clients{
		UserClient: u,
	}
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}
