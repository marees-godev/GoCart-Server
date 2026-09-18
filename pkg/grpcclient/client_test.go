package grpcclient

import (
	"context"
	"net"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/protobuf/inventory"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
	"google.golang.org/grpc/test/bufconn"
)

type mockInventoryServer struct {
	inventory.UnimplementedInventoryServiceServer
	lastReqID string
	callCount int
}

func (s *mockInventoryServer) GetStock(ctx context.Context, req *inventory.GetStockRequest) (*inventory.GetStockResponse, error) {
	s.callCount++
	if md, ok := metadata.FromIncomingContext(ctx); ok {
		if ids := md.Get("x-request-id"); len(ids) > 0 {
			s.lastReqID = ids[0]
		}
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

func TestServiceToService_ConnPool_ReuseAndPropagation(t *testing.T) {
	lis := bufconn.Listen(1024 * 1024)
	mockServer := &mockInventoryServer{}
	grpcServer := grpc.NewServer()
	inventory.RegisterInventoryServiceServer(grpcServer, mockServer)

	go func() {
		_ = grpcServer.Serve(lis)
	}()
	defer grpcServer.Stop()

	dialer := func(context.Context, string) (net.Conn, error) {
		return lis.Dial()
	}

	pool := NewConnPool()
	defer pool.Close()

	// 1. Get first connection
	conn1, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to get connection 1: %v", err)
	}

	// 2. Get second connection to same target -> must return the exact same pointer
	conn2, err := pool.GetConn("passthrough://bufnet", 2*time.Second, grpc.WithContextDialer(dialer))
	if err != nil {
		t.Fatalf("failed to get connection 2: %v", err)
	}

	if conn1 != conn2 {
		t.Errorf("expected connection reuse (same pointer), got different connections")
	}

	// 3. Make RPC call and verify context propagation
	client := inventory.NewInventoryServiceClient(conn1)
	reqCtx := logger.WithRequestID(context.Background(), "trace-s2s-order-to-inventory")

	res, err := client.GetStock(reqCtx, &inventory.GetStockRequest{ProductId: "prod-1"})
	if err != nil {
		t.Fatalf("unexpected RPC error: %v", err)
	}
	if res.Stock.AvailableQuantity != 100 {
		t.Errorf("expected 100 available stock, got %d", res.Stock.AvailableQuantity)
	}
	if mockServer.lastReqID != "trace-s2s-order-to-inventory" {
		t.Errorf("expected request id propagated, got %q", mockServer.lastReqID)
	}

	// 4. Test error translation on service-to-service call
	_, err = client.GetStock(context.Background(), &inventory.GetStockRequest{ProductId: "prod-out-of-stock"})
	if err == nil {
		t.Fatalf("expected error for out of stock product")
	}
	translated := TranslateGRPCError(err)
	if translated == nil {
		t.Fatalf("expected translated error")
	}
}
