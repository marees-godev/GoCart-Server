package grpc

import (
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	categorypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type Clients struct {
	AuthClient      auth.AuthServiceClient
	UserClient      userpb.UserServiceClient
	StoreClient     store.StoreServiceClient
	CategoryClient  categorypb.CategoryServiceClient
	CartClient      cartpb.CartServiceClient
	InventoryClient inventory.InventoryServiceClient
	conns           []*grpc.ClientConn
}

func NewClients(cfg *config.Config, extraOpts ...grpc.DialOption) (*Clients, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(grpcclient.UnaryClientInterceptor(cfg.GRPC.DefaultTimeout)),
		grpc.WithDefaultCallOptions(
			grpc.MaxCallRecvMsgSize(10*1024*1024),
			grpc.MaxCallSendMsgSize(10*1024*1024),
		),
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

	categoryAddr := cfg.GRPC.CategoryServiceAddr
	if categoryAddr == "" {
		categoryAddr = "localhost:50054"
	}
	categoryConn, err := grpc.NewClient(categoryAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		orderConn.Close()
		return nil, err
	}

	storeAddr := cfg.GRPC.StoreServiceAddr
	if storeAddr == "" {
		storeAddr = "localhost:50055"
	}
	storeConn, err := grpc.NewClient(storeAddr, opts...)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		orderConn.Close()
		categoryConn.Close()
		return nil, err
	}

	invAddr := cfg.GRPC.InventoryServiceAddr
	if invAddr == "" {
		invAddr = "localhost:50058"
	}
	invConn, err := grpc.NewClient(invAddr, opts...)
	if err != nil {
		userConn.Close()
		productConn.Close()
		cartConn.Close()
		orderConn.Close()
		storeConn.Close()
		return nil, err
	}

	return &Clients{
		AuthClient:      auth.NewAuthServiceClient(authConn),
		UserClient:      userpb.NewUserServiceClient(userConn),
		StoreClient:     store.NewStoreServiceClient(storeConn),
		CategoryClient:  categorypb.NewCategoryServiceClient(categoryConn),
		CartClient:      cartpb.NewCartServiceClient(cartConn),
		InventoryClient: inventory.NewInventoryServiceClient(invConn),
		conns: []*grpc.ClientConn{
			authConn, userConn, productConn, cartConn, orderConn, storeConn, categoryConn, invConn,
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
		if cat, ok := svc.(categorypb.CategoryServiceClient); ok {
			c.CategoryClient = cat
		}
		if inv, ok := svc.(inventory.InventoryServiceClient); ok {
			c.InventoryClient = inv
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
