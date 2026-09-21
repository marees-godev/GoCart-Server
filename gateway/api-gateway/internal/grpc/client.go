package grpc

import (
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc/pb/userpb"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	AuthClient auth.AuthServiceClient
	UserClient  userpb.UserServiceClient
	StoreClient store.StoreServiceClient
	conns       []*grpc.ClientConn
}

func NewClients(cfg *config.Config, extraOpts ...grpc.DialOption) (*Clients, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcclient.UnaryClientInterceptor(cfg.GRPC.DefaultTimeout)),
	}
	opts = append(opts, extraOpts...)

	authAddr := cfg.GRPC.AuthServiceAddr
	if authAddr == "" {
		authAddr = "localhost:50051"
	}
	authConn, err := grpc.NewClient(authAddr, opts...)
	if err != nil {
		return nil, err
	}

	userConn, err := grpc.NewClient(cfg.UserServiceAddr, opts...)
	if err != nil {
		authConn.Close()
		return nil, err
	}

	productConn, err := grpc.NewClient(cfg.ProductServiceAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		return nil, err
	}

	cartConn, err := grpc.NewClient(cfg.CartServiceAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		return nil, err
	}

	orderConn, err := grpc.NewClient(cfg.OrderServiceAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		return nil, err
	}

	storeConn, err := grpc.NewClient(cfg.GRPC.StoreServiceAddr, opts...)
	if err != nil {
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		orderConn.Close()
		return nil, err
	}

	return &Clients{
		AuthClient: auth.NewAuthServiceClient(authConn),
		UserClient:  userpb.NewUserServiceClient(userConn),
		StoreClient: store.NewStoreServiceClient(storeConn),
		conns: []*grpc.ClientConn{
			authConn, userConn, productConn, cartConn, orderConn, storeConn,
		},
	}, nil
}

func NewClientsWithServices(
	u userpb.UserServiceClient,
	extraServices ...any,
) *Clients {
	c := &Clients{
		UserClient: u,
	}
	for _, svc := range extraServices {
		if s, ok := svc.(store.StoreServiceClient); ok {
			c.StoreClient = s
		}
		if a, ok := svc.(auth.AuthServiceClient); ok {
			c.AuthClient = a
		}
	}
	return c
}

func (c *Clients) Close() {
	for _, conn := range c.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
}
