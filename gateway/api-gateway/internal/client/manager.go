package client

import (
	"sync"

	"github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/category"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/notification"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/rating"
	returns "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ClientManager manages persistent, reused gRPC client connections to all internal microservices.
type ClientManager struct {
	mu                 sync.Mutex
	conns              []*grpc.ClientConn
	AuthClient         auth.AuthServiceClient
	UserClient         user.UserServiceClient
	ProductClient      product.ProductServiceClient
	CategoryClient     category.CategoryServiceClient
	StoreClient        store.StoreServiceClient
	MerchantClient     merchant.MerchantServiceClient
	CartClient         cart.CartServiceClient
	InventoryClient    inventory.InventoryServiceClient
	OrderClient        order.OrderServiceClient
	PaymentClient      payment.PaymentServiceClient
	DeliveryClient     delivery.DeliveryServiceClient
	ReturnClient       returns.ReturnServiceClient
	RatingClient       rating.RatingServiceClient
	NotificationClient notification.NotificationServiceClient
}

// NewClientManager initializes persistent gRPC connections for all services using configuration.
func NewClientManager(cfg *config.Config, extraOpts ...grpc.DialOption) (*ClientManager, error) {
	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor(cfg.GRPC.DefaultTimeout)),
	}
	opts = append(opts, extraOpts...)

	dialService := func(addr string) (*grpc.ClientConn, error) {
		return grpc.NewClient(addr, opts...)
	}

	authConn, err := dialService(cfg.GRPC.AuthServiceAddr)
	if err != nil {
		return nil, err
	}
	userConn, err := dialService(cfg.GRPC.UserServiceAddr)
	if err != nil {
		authConn.Close()
		return nil, err
	}
	productConn, err := dialService(cfg.GRPC.ProductServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		return nil, err
	}
	categoryConn, err := dialService(cfg.GRPC.CategoryServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		return nil, err
	}
	storeConn, err := dialService(cfg.GRPC.StoreServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		return nil, err
	}
	merchantConn, err := dialService(cfg.GRPC.MerchantServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		return nil, err
	}
	cartConn, err := dialService(cfg.GRPC.CartServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		return nil, err
	}
	inventoryConn, err := dialService(cfg.GRPC.InventoryServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		return nil, err
	}
	orderConn, err := dialService(cfg.GRPC.OrderServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		return nil, err
	}
	paymentConn, err := dialService(cfg.GRPC.PaymentServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		orderConn.Close()
		return nil, err
	}
	deliveryConn, err := dialService(cfg.GRPC.DeliveryServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		orderConn.Close()
		paymentConn.Close()
		return nil, err
	}
	returnConn, err := dialService(cfg.GRPC.ReturnServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		orderConn.Close()
		paymentConn.Close()
		deliveryConn.Close()
		return nil, err
	}
	ratingConn, err := dialService(cfg.GRPC.RatingServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		orderConn.Close()
		paymentConn.Close()
		deliveryConn.Close()
		returnConn.Close()
		return nil, err
	}
	notificationConn, err := dialService(cfg.GRPC.NotificationServiceAddr)
	if err != nil {
		authConn.Close()
		userConn.Close()
		productConn.Close()
		categoryConn.Close()
		storeConn.Close()
		merchantConn.Close()
		cartConn.Close()
		inventoryConn.Close()
		orderConn.Close()
		paymentConn.Close()
		deliveryConn.Close()
		returnConn.Close()
		ratingConn.Close()
		return nil, err
	}

	conns := []*grpc.ClientConn{
		authConn, userConn, productConn, categoryConn, storeConn,
		merchantConn, cartConn, inventoryConn, orderConn, paymentConn,
		deliveryConn, returnConn, ratingConn, notificationConn,
	}

	return &ClientManager{
		conns:              conns,
		AuthClient:         auth.NewAuthServiceClient(authConn),
		UserClient:         user.NewUserServiceClient(userConn),
		ProductClient:      product.NewProductServiceClient(productConn),
		CategoryClient:     category.NewCategoryServiceClient(categoryConn),
		StoreClient:        store.NewStoreServiceClient(storeConn),
		MerchantClient:     merchant.NewMerchantServiceClient(merchantConn),
		CartClient:         cart.NewCartServiceClient(cartConn),
		InventoryClient:    inventory.NewInventoryServiceClient(inventoryConn),
		OrderClient:        order.NewOrderServiceClient(orderConn),
		PaymentClient:      payment.NewPaymentServiceClient(paymentConn),
		DeliveryClient:     delivery.NewDeliveryServiceClient(deliveryConn),
		ReturnClient:       returns.NewReturnServiceClient(returnConn),
		RatingClient:       rating.NewRatingServiceClient(ratingConn),
		NotificationClient: notification.NewNotificationServiceClient(notificationConn),
	}, nil
}

// NewClientManagerWithServices creates a ClientManager with pre-configured clients (useful for testing).
func NewClientManagerWithServices(cartClient cart.CartServiceClient) *ClientManager {
	return &ClientManager{
		CartClient: cartClient,
	}
}

// Close gracefully closes all active gRPC connections.
func (cm *ClientManager) Close() {
	cm.mu.Lock()
	defer cm.mu.Unlock()

	for _, conn := range cm.conns {
		if conn != nil {
			_ = conn.Close()
		}
	}
	cm.conns = nil
}
