package grpcclient

import (
	"sync"
	"time"

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
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// ConnPool manages cached, reused gRPC connections for inter-service communication.
type ConnPool struct {
	mu    sync.Mutex
	conns map[string]*grpc.ClientConn
}

// NewConnPool creates a new thread-safe connection pool.
func NewConnPool() *ConnPool {
	return &ConnPool{
		conns: make(map[string]*grpc.ClientConn),
	}
}

// GetConn returns an existing connection to target or creates a new persistent one.
func (p *ConnPool) GetConn(target string, defaultTimeout time.Duration, extraOpts ...grpc.DialOption) (*grpc.ClientConn, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if conn, ok := p.conns[target]; ok && conn != nil {
		return conn, nil
	}

	opts := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(UnaryClientInterceptor(defaultTimeout)),
	}
	opts = append(opts, extraOpts...)

	conn, err := grpc.NewClient(target, opts...)
	if err != nil {
		return nil, err
	}

	p.conns[target] = conn
	return conn, nil
}

// Close closes all cached connections in the pool.
func (p *ConnPool) Close() {
	p.mu.Lock()
	defer p.mu.Unlock()

	for target, conn := range p.conns {
		if conn != nil {
			_ = conn.Close()
		}
		delete(p.conns, target)
	}
}

// Service-to-service client constructors:

func NewAuthClient(target string, timeout time.Duration, opts ...grpc.DialOption) (auth.AuthServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return auth.NewAuthServiceClient(conn), conn, nil
}

func NewUserClient(target string, timeout time.Duration, opts ...grpc.DialOption) (user.UserServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return user.NewUserServiceClient(conn), conn, nil
}

func NewProductClient(target string, timeout time.Duration, opts ...grpc.DialOption) (product.ProductServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return product.NewProductServiceClient(conn), conn, nil
}

func NewCategoryClient(target string, timeout time.Duration, opts ...grpc.DialOption) (category.CategoryServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return category.NewCategoryServiceClient(conn), conn, nil
}

func NewStoreClient(target string, timeout time.Duration, opts ...grpc.DialOption) (store.StoreServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return store.NewStoreServiceClient(conn), conn, nil
}

func NewMerchantClient(target string, timeout time.Duration, opts ...grpc.DialOption) (merchant.MerchantServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return merchant.NewMerchantServiceClient(conn), conn, nil
}

func NewCartClient(target string, timeout time.Duration, opts ...grpc.DialOption) (cart.CartServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return cart.NewCartServiceClient(conn), conn, nil
}

func NewInventoryClient(target string, timeout time.Duration, opts ...grpc.DialOption) (inventory.InventoryServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return inventory.NewInventoryServiceClient(conn), conn, nil
}

func NewOrderClient(target string, timeout time.Duration, opts ...grpc.DialOption) (order.OrderServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return order.NewOrderServiceClient(conn), conn, nil
}

func NewPaymentClient(target string, timeout time.Duration, opts ...grpc.DialOption) (payment.PaymentServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return payment.NewPaymentServiceClient(conn), conn, nil
}

func NewDeliveryClient(target string, timeout time.Duration, opts ...grpc.DialOption) (delivery.DeliveryServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return delivery.NewDeliveryServiceClient(conn), conn, nil
}

func NewReturnClient(target string, timeout time.Duration, opts ...grpc.DialOption) (returns.ReturnServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return returns.NewReturnServiceClient(conn), conn, nil
}

func NewRatingClient(target string, timeout time.Duration, opts ...grpc.DialOption) (rating.RatingServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return rating.NewRatingServiceClient(conn), conn, nil
}

func NewNotificationClient(target string, timeout time.Duration, opts ...grpc.DialOption) (notification.NotificationServiceClient, *grpc.ClientConn, error) {
	pool := NewConnPool()
	conn, err := pool.GetConn(target, timeout, opts...)
	if err != nil {
		return nil, nil, err
	}
	return notification.NewNotificationServiceClient(conn), conn, nil
}

// MapAppErrorToGRPC converts an application error into an equivalent gRPC status error.
func MapAppErrorToGRPC(err error) error {
	return errors.MapAppErrorToGRPC(err)
}

// ToGRPC converts an application error into an equivalent gRPC status error.
func ToGRPC(err error) error {
	return errors.MapAppErrorToGRPC(err)
}
