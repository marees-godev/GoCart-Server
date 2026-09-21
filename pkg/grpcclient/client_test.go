package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockInventoryServer struct {
	inventory.UnimplementedInventoryServiceServer
	lastReqID   string
	lastUserID  string
	lastRole    string
	lastEmail   string
	callCount   int
	delay       time.Duration
	returnError error
}

func (s *mockInventoryServer) GetStock(ctx context.Context, req *inventory.GetStockRequest) (*inventory.GetStockResponse, error) {
	s.callCount++
	s.lastUserID = GetUserID(ctx)
	s.lastRole = GetUserRole(ctx)
	s.lastReqID = middlewareGetRequestID(ctx)

	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get(HeaderRequestID); len(ids) > 0 && s.lastReqID == "" {
			s.lastReqID = ids[0]
		}
		if uids := md.Get(HeaderUserID); len(uids) > 0 && s.lastUserID == "" {
			s.lastUserID = uids[0]
		}
		if roles := md.Get(HeaderUserRole); len(roles) > 0 && s.lastRole == "" {
			s.lastRole = roles[0]
		}
		if emails := md.Get(HeaderUserEmail); len(emails) > 0 {
			s.lastEmail = emails[0]
		}
	}

	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}

	if s.returnError != nil {
		return nil, s.returnError
	}

	if req.ProductId == "prod-out-of-stock" {
		return nil, status.Error(codes.NotFound, "product not found in inventory")
	}

	return &inventory.GetStockResponse{
		Stock: &inventory.StockItem{
			ProductId:         req.ProductId,
			AvailableQuantity: 100,
			ReservedQuantity:  10,
		},
	}, nil
}

func middlewareGetRequestID(ctx context.Context) string {
	if ctx == nil {
		return ""
	}
	if id, ok := ctx.Value(logger.RequestIDKey).(string); ok {
		return id
	}
	return ""
}

func setupBufconnServer(t *testing.T, srv *mockInventoryServer) (*grpc.Server, *bufconn.Listener) {
	t.Helper()
	lis := bufconn.Listen(1024 * 1024)
	s := grpc.NewServer(grpc.UnaryInterceptor(UnaryServerInterceptor()))
	inventory.RegisterInventoryServiceServer(s, srv)

	go func() {
		_ = s.Serve(lis)
	}()

	return s, lis
}

func TestContextPropagation_AuthenticatedUser(t *testing.T) {
	mockSrv := &mockInventoryServer{}
	server, lis := setupBufconnServer(t, mockSrv)
	defer server.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	pool := NewConnPool()
	defer pool.Close()

	conn, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := inventory.NewInventoryServiceClient(conn)

	// Set authenticated user and request ID
	ctx := context.Background()
	ctx = logger.WithRequestID(ctx, "req-12345")
	ctx = auth.WithUser(ctx, &auth.UserContext{
		UserID: "user-42",
		Role:   "admin",
		Email:  "admin@gocart.com",
	})

	res, err := client.GetStock(ctx, &inventory.GetStockRequest{ProductId: "prod-1"})
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	if res.Stock.AvailableQuantity != 100 {
		t.Errorf("expected 100 stock, got %d", res.Stock.AvailableQuantity)
	}

	if mockSrv.lastUserID != "user-42" {
		t.Errorf("expected user id 'user-42', got %q", mockSrv.lastUserID)
	}
	if mockSrv.lastRole != "admin" {
		t.Errorf("expected role 'admin', got %q", mockSrv.lastRole)
	}
	if mockSrv.lastReqID != "req-12345" {
		t.Errorf("expected req id 'req-12345', got %q", mockSrv.lastReqID)
	}
	if mockSrv.lastEmail != "admin@gocart.com" {
		t.Errorf("expected email 'admin@gocart.com', got %q", mockSrv.lastEmail)
	}
}

func TestContextPropagation_UntrustedClientIdentityStripped(t *testing.T) {
	mockSrv := &mockInventoryServer{}
	server, lis := setupBufconnServer(t, mockSrv)
	defer server.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	pool := NewConnPool()
	defer pool.Close()

	conn, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := inventory.NewInventoryServiceClient(conn)

	// Case 1: Unauthenticated request attempting to inject identity via outgoing metadata
	spoofedMD := metadata.Pairs(
		HeaderUserID, "hacked-user-id",
		HeaderUserRole, "admin",
		HeaderUserEmail, "evil@attacker.com",
	)
	unauthCtx := metadata.NewOutgoingContext(context.Background(), spoofedMD)

	_, err = client.GetStock(unauthCtx, &inventory.GetStockRequest{ProductId: "prod-1"})
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	if mockSrv.lastUserID != "" {
		t.Errorf("untrusted user ID was NOT stripped, got %q", mockSrv.lastUserID)
	}
	if mockSrv.lastRole != "" {
		t.Errorf("untrusted user role was NOT stripped, got %q", mockSrv.lastRole)
	}

	// Case 2: Authenticated request where client attempts to override authenticated user with forged identity
	authCtx := auth.WithUser(unauthCtx, &auth.UserContext{
		UserID: "trusted-user-1",
		Role:   "customer",
		Email:  "trusted@user.com",
	})

	_, err = client.GetStock(authCtx, &inventory.GetStockRequest{ProductId: "prod-1"})
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	if mockSrv.lastUserID != "trusted-user-1" {
		t.Errorf("expected authenticated user id 'trusted-user-1', got %q", mockSrv.lastUserID)
	}
	if mockSrv.lastRole != "customer" {
		t.Errorf("expected authenticated role 'customer', got %q", mockSrv.lastRole)
	}
}

func TestContextPropagation_MissingRequestIDGenerated(t *testing.T) {
	mockSrv := &mockInventoryServer{}
	server, lis := setupBufconnServer(t, mockSrv)
	defer server.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	pool := NewConnPool()
	defer pool.Close()

	conn, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := inventory.NewInventoryServiceClient(conn)

	// Call without any request ID in context
	_, err = client.GetStock(context.Background(), &inventory.GetStockRequest{ProductId: "prod-1"})
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}

	if mockSrv.lastReqID == "" {
		t.Fatalf("expected auto-generated request ID, got empty string")
	}
	if _, err := uuid.Parse(mockSrv.lastReqID); err != nil {
		t.Errorf("generated request ID is not a valid UUID: %q (%v)", mockSrv.lastReqID, err)
	}
}

func TestContextPropagation_Cancellation(t *testing.T) {
	mockSrv := &mockInventoryServer{
		delay: 500 * time.Millisecond,
	}
	server, lis := setupBufconnServer(t, mockSrv)
	defer server.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	pool := NewConnPool()
	defer pool.Close()

	conn, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to dial: %v", err)
	}

	client := inventory.NewInventoryServiceClient(conn)

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	_, err = client.GetStock(ctx, &inventory.GetStockRequest{ProductId: "prod-1"})
	if err == nil {
		t.Fatalf("expected error due to cancellation, got nil")
	}

	st, ok := status.FromError(err)
	if !ok || (st.Code() != codes.Canceled && st.Code() != codes.DeadlineExceeded) {
		t.Errorf("expected Canceled status code, got: %v (code: %v)", err, st.Code())
	}
}

