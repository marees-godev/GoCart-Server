package client

import (
	"context"
	"net"
	"testing"
	"time"

	cartpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/cart"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/config"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockCartServer struct {
	cartpb.UnimplementedCartServiceServer
	lastReqID   string
	callCount   int
	delay       time.Duration
	returnError error
}

func (s *mockCartServer) GetCart(ctx context.Context, req *cartpb.GetCartRequest) (*cartpb.GetCartResponse, error) {
	s.callCount++
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get("x-request-id"); len(ids) > 0 {
			s.lastReqID = ids[0]
		}
	}

	if s.delay > 0 {
		time.Sleep(s.delay)
	}

	if s.returnError != nil {
		return nil, s.returnError
	}

	return &cartpb.GetCartResponse{
		Cart: &cartpb.Cart{
			Id:          "cart-123",
			UserId:      req.UserId,
			TotalAmount: 50.0,
			Items: []*cartpb.CartItem{
				{
					Id:        "item-1",
					ProductId: "prod-1",
					Quantity:  2,
					UnitPrice: 25.0,
				},
			},
		},
	}, nil
}

func setupBufconnServer(t *testing.T, srv *mockCartServer) (*grpc.Server, *bufconn.Listener) {
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer()
	cartpb.RegisterCartServiceServer(s, srv)

	go func() {
		if err := s.Serve(lis); err != nil && err != grpc.ErrServerStopped {
			t.Logf("bufconn server closed: %v", err)
		}
	}()

	return s, lis
}

func TestClientManager_ConnectionReuseAndContextPropagation(t *testing.T) {
	mockSrv := &mockCartServer{}
	grpcServer, lis := setupBufconnServer(t, mockSrv)
	defer grpcServer.Stop()

	cfg := &config.Config{
		GRPC: config.GRPCConfig{
			CartServiceAddr: "passthrough://bufnet",
			DefaultTimeout:  2 * time.Second,
		},
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	cm, err := NewClientManager(cfg, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to create client manager: %v", err)
	}
	defer cm.Close()

	// 1. Verify Call 1 propagates Request ID via logger context
	reqCtx := logger.WithRequestID(context.Background(), "req-xyz-999")

	res1, err := cm.CartClient.GetCart(reqCtx, &cartpb.GetCartRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error on call 1: %v", err)
	}
	if res1.Cart.Id != "cart-123" {
		t.Errorf("expected cart-123, got %s", res1.Cart.Id)
	}
	if mockSrv.lastReqID != "req-xyz-999" {
		t.Errorf("expected request id 'req-xyz-999', got %q", mockSrv.lastReqID)
	}

	// 2. Verify Call 2 reuses the same connection
	res2, err := cm.CartClient.GetCart(context.Background(), &cartpb.GetCartRequest{UserId: "u1"})
	if err != nil {
		t.Fatalf("unexpected error on call 2: %v", err)
	}
	if res2.Cart.Id != "cart-123" {
		t.Errorf("expected cart-123, got %s", res2.Cart.Id)
	}

	if mockSrv.callCount != 2 {
		t.Errorf("expected 2 calls, got %d", mockSrv.callCount)
	}
}

func TestClientManager_TimeoutEnforcement(t *testing.T) {
	mockSrv := &mockCartServer{
		delay: 100 * time.Millisecond,
	}
	grpcServer, lis := setupBufconnServer(t, mockSrv)
	defer grpcServer.Stop()

	// Configure a short timeout
	cfg := &config.Config{
		GRPC: config.GRPCConfig{
			CartServiceAddr: "passthrough://bufnet",
			DefaultTimeout:  20 * time.Millisecond,
		},
	}

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	cm, err := NewClientManager(cfg, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to create client manager: %v", err)
	}
	defer cm.Close()

	_, err = cm.CartClient.GetCart(context.Background(), &cartpb.GetCartRequest{UserId: "u1"})
	if err == nil {
		t.Fatalf("expected timeout error, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.DeadlineExceeded {
		t.Errorf("expected DeadlineExceeded error code, got: %v", err)
	}

	translated := TranslateGRPCError(err)
	if translated == nil {
		t.Fatalf("expected translated error")
	}
}
