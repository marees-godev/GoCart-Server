package grpc

import (
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	AuthClient  auth.AuthServiceClient
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

	userAddr := cfg.GRPC.UserServiceAddr
	if userAddr == "" {
		userAddr = cfg.UserServiceAddr
	}
	if userAddr == "" {
		userAddr = "localhost:50052"
	}
	userConn, err := grpc.NewClient(userAddr, opts...)
	if err != nil {
		authConn.Close()
		return nil, err
	}

	productAddr := cfg.GRPC.ProductServiceAddr
	if productAddr == "" {
		productAddr = cfg.ProductServiceAddr
	}
	if productAddr == "" {
		productAddr = "localhost:50053"
	}
	productConn, err := grpc.NewClient(productAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		return nil, err
	}

	cartAddr := cfg.GRPC.CartServiceAddr
	if cartAddr == "" {
		cartAddr = cfg.CartServiceAddr
	}
	if cartAddr == "" {
		cartAddr = "localhost:50057"
	}
	cartConn, err := grpc.NewClient(cartAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		return nil, err
	}

	orderAddr := cfg.GRPC.OrderServiceAddr
	if orderAddr == "" {
		orderAddr = cfg.OrderServiceAddr
	}
	if orderAddr == "" {
		orderAddr = "localhost:50059"
	}
	orderConn, err := grpc.NewClient(orderAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		return nil, err
	}

	storeAddr := cfg.GRPC.StoreServiceAddr
	if storeAddr == "" {
		storeAddr = "localhost:50055"
	}
	storeConn, err := grpc.NewClient(storeAddr, opts...)
	if err != nil {
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		orderConn.Close()
		return nil, err
	}

	return &Clients{
		AuthClient:  auth.NewAuthServiceClient(authConn),
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
