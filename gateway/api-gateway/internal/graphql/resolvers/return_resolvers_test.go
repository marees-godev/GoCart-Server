package resolvers

import (
	"context"
	"testing"

	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
)

type mockReturnClient struct {
	returnpb.ReturnServiceClient
	lastRequestReq   *returnpb.RequestReturnRequest
	lastVerifyReq    *returnpb.VerifyProductRequest
	lastUpdateReq    *returnpb.UpdateReturnStatusRequest
	lastGetReq       *returnpb.GetReturnRequest
	lastGetByOrder   *returnpb.GetReturnsByOrderRequest
	lastListReq      *returnpb.ListReturnsRequest
}

func (m *mockReturnClient) RequestReturn(ctx context.Context, in *returnpb.RequestReturnRequest, opts ...grpcPkg.CallOption) (*returnpb.RequestReturnResponse, error) {
	m.lastRequestReq = in
	return &returnpb.RequestReturnResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:           "ret-100",
			OrderId:      in.OrderId,
			OrderItemId:  in.OrderItemId,
			UserId:       in.UserId,
			Reason:       in.Reason,
			Status:       "REQUESTED",
			Type:         in.Type,
			RefundAmount: 99.99,
			RequestedAt:  "2026-10-26T10:00:00Z",
		},
	}, nil
}

func (m *mockReturnClient) VerifyProduct(ctx context.Context, in *returnpb.VerifyProductRequest, opts ...grpcPkg.CallOption) (*returnpb.VerifyProductResponse, error) {
	m.lastVerifyReq = in
	status := "VERIFIED"
	if !in.IsApproved {
		status = "REJECTED"
	}
	return &returnpb.VerifyProductResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:           in.ReturnId,
			OrderId:      "order-1",
			UserId:       "user-1",
			Reason:       "Defective",
			Status:       status,
			Type:         "RETURN",
			RefundAmount: 99.99,
			Verification: &returnpb.ProductVerification{
				Id:          "ver-1",
				ReturnId:    in.ReturnId,
				VerifiedBy:  in.VerifiedBy,
				Condition:   in.Condition,
				IsApproved:  in.IsApproved,
				ProofImages: in.ProofImages,
				Notes:       in.Notes,
				VerifiedAt:  "2026-10-26T12:00:00Z",
			},
		},
		Verification: &returnpb.ProductVerification{
			Id:          "ver-1",
			ReturnId:    in.ReturnId,
			VerifiedBy:  in.VerifiedBy,
			Condition:   in.Condition,
			IsApproved:  in.IsApproved,
			ProofImages: in.ProofImages,
			Notes:       in.Notes,
			VerifiedAt:  "2026-10-26T12:00:00Z",
		},
	}, nil
}

func (m *mockReturnClient) UpdateReturnStatus(ctx context.Context, in *returnpb.UpdateReturnStatusRequest, opts ...grpcPkg.CallOption) (*returnpb.UpdateReturnStatusResponse, error) {
	m.lastUpdateReq = in
	return &returnpb.UpdateReturnStatusResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:     in.Id,
			Status: in.Status,
		},
	}, nil
}

func (m *mockReturnClient) GetReturn(ctx context.Context, in *returnpb.GetReturnRequest, opts ...grpcPkg.CallOption) (*returnpb.GetReturnResponse, error) {
	m.lastGetReq = in
	return &returnpb.GetReturnResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:           in.Id,
			OrderId:      "order-1",
			UserId:       "user-1",
			Reason:       "Damaged item",
			Status:       "REQUESTED",
			Type:         "RETURN",
			RefundAmount: 50.0,
		},
	}, nil
}

func (m *mockReturnClient) GetReturnsByOrder(ctx context.Context, in *returnpb.GetReturnsByOrderRequest, opts ...grpcPkg.CallOption) (*returnpb.GetReturnsByOrderResponse, error) {
	m.lastGetByOrder = in
	return &returnpb.GetReturnsByOrderResponse{
		Returns: []*returnpb.ReturnOrder{
			{
				Id:      "ret-1",
				OrderId: in.OrderId,
				Status:  "REQUESTED",
			},
		},
	}, nil
}

func (m *mockReturnClient) ListReturns(ctx context.Context, in *returnpb.ListReturnsRequest, opts ...grpcPkg.CallOption) (*returnpb.ListReturnsResponse, error) {
	m.lastListReq = in
	return &returnpb.ListReturnsResponse{
		Returns: []*returnpb.ReturnOrder{
			{
				Id:     "ret-1",
				UserId: in.UserId,
				Status: in.Status,
			},
		},
		Total: 1,
	}, nil
}

func TestRequestReturn_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	orderItemID := "item-1"
	retType := model.ReturnTypeReturn
	input := model.RequestReturnInput{
		OrderID:     "order-456",
		OrderItemID: &orderItemID,
		Reason:      "Wrong size",
		Type:        &retType,
	}

	res, err := resolver.Mutation().RequestReturn(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "ret-100" {
		t.Errorf("expected ID 'ret-100', got '%s'", res.ID)
	}
	if mockClient.lastRequestReq.UserId != "user-123" {
		t.Errorf("expected user id 'user-123', got '%s'", mockClient.lastRequestReq.UserId)
	}
	if mockClient.lastRequestReq.Type != "RETURN" {
		t.Errorf("expected type 'RETURN', got '%s'", mockClient.lastRequestReq.Type)
	}
}

func TestRequestReplacement_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	orderItemID := "item-1"
	input := model.RequestReplacementInput{
		OrderID:     "order-456",
		OrderItemID: &orderItemID,
		Reason:      "Item defective, need replacement",
	}

	res, err := resolver.Mutation().RequestReplacement(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "ret-100" {
		t.Errorf("expected ID 'ret-100', got '%s'", res.ID)
	}
	if mockClient.lastRequestReq.Type != "REPLACEMENT" {
		t.Errorf("expected type 'REPLACEMENT', got '%s'", mockClient.lastRequestReq.Type)
	}
}

func TestRequestReturn_Unauthorized(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := context.Background()
	input := model.RequestReturnInput{
		OrderID: "order-456",
		Reason:  "Defective",
	}

	_, err := resolver.Mutation().RequestReturn(ctx, input)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

func TestVerifyProduct_Success_DeliveryPartner(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "user-dp-1",
		PartnerID: "partner-999",
		Role:      "DELIVERY_PARTNER",
	})

	notes := "Product is defective but packaging is intact"
	input := model.VerifyProductInput{
		ReturnID:    "ret-100",
		Condition:   "DEFECTIVE",
		IsApproved:  true,
		ProofImages: []string{"https://storage.gocart.internal/proof/1.jpg"},
		Notes:       &notes,
	}

	res, err := resolver.Mutation().VerifyProduct(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "VERIFIED" {
		t.Errorf("expected status 'VERIFIED', got '%s'", res.Status)
	}
	if res.Verification == nil {
		t.Fatal("expected verification details, got nil")
	}
	if res.Verification.VerifiedBy != "partner-999" {
		t.Errorf("expected verifiedBy 'partner-999', got '%s'", res.Verification.VerifiedBy)
	}
	if len(res.Verification.ProofImages) != 1 {
		t.Errorf("expected 1 proof image, got %d", len(res.Verification.ProofImages))
	}
}

func TestVerifyProduct_Success_WithoutImages(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "user-dp-1",
		PartnerID: "partner-999",
		Role:      "DELIVERY_PARTNER",
	})

	input := model.VerifyProductInput{
		ReturnID:   "ret-100",
		Condition:  "GOOD",
		IsApproved: true,
	}

	res, err := resolver.Mutation().VerifyProduct(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error when verifying without images: %v", err)
	}
	if res.Status != "VERIFIED" {
		t.Errorf("expected status 'VERIFIED', got '%s'", res.Status)
	}
}

func TestVerifyProduct_Success_WithMultipleImages(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "user-dp-1",
		PartnerID: "partner-999",
		Role:      "DELIVERY_PARTNER",
	})

	images := []string{
		"https://storage.gocart.internal/proof/img1.jpg",
		"https://storage.gocart.internal/proof/img2.jpg",
	}
	input := model.VerifyProductInput{
		ReturnID:    "ret-100",
		Condition:   "DEFECTIVE",
		IsApproved:  true,
		ProofImages: images,
	}

	res, err := resolver.Mutation().VerifyProduct(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error when verifying with multiple images: %v", err)
	}
	if res.Status != "VERIFIED" {
		t.Errorf("expected status 'VERIFIED', got '%s'", res.Status)
	}
	if res.Verification == nil {
		t.Fatal("expected verification details, got nil")
	}
	if len(res.Verification.ProofImages) != 2 {
		t.Errorf("expected 2 proof images, got %d", len(res.Verification.ProofImages))
	}
}

func TestVerifyProduct_ForbiddenCustomer(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	input := model.VerifyProductInput{
		ReturnID:    "ret-100",
		Condition:   "GOOD",
		IsApproved:  true,
		ProofImages: []string{"https://img.jpg"},
	}

	_, err := resolver.Mutation().VerifyProduct(ctx, input)
	if err == nil {
		t.Fatal("expected forbidden error for customer, got nil")
	}
}

func TestUpdateReturnStatus_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	res, err := resolver.Mutation().UpdateReturnStatus(ctx, "ret-100", "APPROVED")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "APPROVED" {
		t.Errorf("expected status 'APPROVED', got '%s'", res.Status)
	}
}

func TestReturnOrder_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := context.Background()
	res, err := resolver.Query().ReturnOrder(ctx, "ret-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "ret-100" {
		t.Errorf("expected ID 'ret-100', got '%s'", res.ID)
	}
}

func TestReturnOrdersByOrder_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := context.Background()
	res, err := resolver.Query().ReturnOrdersByOrder(ctx, "order-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 1 {
		t.Fatalf("expected 1 return, got %d", len(res))
	}
	if res[0].OrderID != "order-123" {
		t.Errorf("expected OrderID 'order-123', got '%s'", res[0].OrderID)
	}
}

func TestListReturns_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	limit := 10
	offset := 0
	status := "REQUESTED"
	res, err := resolver.Query().ListReturns(ctx, &limit, &offset, &status)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(res) != 1 {
		t.Fatalf("expected 1 return, got %d", len(res))
	}
	if mockClient.lastListReq.UserId != "user-123" {
		t.Errorf("expected user id 'user-123', got '%s'", mockClient.lastListReq.UserId)
	}
}
