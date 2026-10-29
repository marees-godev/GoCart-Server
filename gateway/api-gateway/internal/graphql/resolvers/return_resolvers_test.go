package resolvers

import (
	"context"
	"testing"

	deliverypb "github.com/marees-godev/GoCart-Server/contracts/protobuf/delivery"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	storepb "github.com/marees-godev/GoCart-Server/contracts/protobuf/store"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
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
	lastCreateExcReq *returnpb.CreateExchangeRequest
	lastGetExcReq    *returnpb.GetExchangeRequest
	lastVerifyExcReq *returnpb.VerifyExchangeProductRequest
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
			Id:              in.Id,
			Status:          in.Status,
			RejectionReason: in.RejectionReason,
		},
	}, nil
}

func (m *mockReturnClient) ApproveReturn(ctx context.Context, in *returnpb.ApproveReturnRequest, opts ...grpcPkg.CallOption) (*returnpb.ApproveReturnResponse, error) {
	return &returnpb.ApproveReturnResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:     in.Id,
			Status: "APPROVED",
		},
	}, nil
}

func (m *mockReturnClient) RejectReturn(ctx context.Context, in *returnpb.RejectReturnRequest, opts ...grpcPkg.CallOption) (*returnpb.RejectReturnResponse, error) {
	return &returnpb.RejectReturnResponse{
		ReturnOrder: &returnpb.ReturnOrder{
			Id:              in.Id,
			Status:          "REJECTED",
			RejectionReason: in.RejectionReason,
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
				UserId:  "user-1",
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

func (m *mockReturnClient) CreateExchange(ctx context.Context, in *returnpb.CreateExchangeRequest, opts ...grpcPkg.CallOption) (*returnpb.CreateExchangeResponse, error) {
	m.lastCreateExcReq = in
	return &returnpb.CreateExchangeResponse{
		Exchange: &returnpb.Exchange{
			Id:                "exc-100",
			OrderId:           in.OrderId,
			OrderItemId:       in.OrderItemId,
			UserId:            in.UserId,
			NewProductId:      in.NewProductId,
			OldBrand:          in.OldBrand,
			OldModel:          in.OldModel,
			BaseExchangeValue: in.BaseExchangeValue,
			EstimatedDiscount: in.BaseExchangeValue,
			Status:            "ESTIMATED",
			Answers:           in.Answers,
		},
	}, nil
}

func (m *mockReturnClient) GetExchange(ctx context.Context, in *returnpb.GetExchangeRequest, opts ...grpcPkg.CallOption) (*returnpb.GetExchangeResponse, error) {
	m.lastGetExcReq = in
	return &returnpb.GetExchangeResponse{
		Exchange: &returnpb.Exchange{
			Id:                "exc-100",
			OrderId:           "order-1",
			OrderItemId:       "item-1",
			UserId:            "user-123",
			NewProductId:      "prod-2",
			OldBrand:          "Apple",
			OldModel:          "iPhone 13",
			BaseExchangeValue: 300.0,
			EstimatedDiscount: 250.0,
			Status:            "ESTIMATED",
		},
	}, nil
}

func (m *mockReturnClient) VerifyExchangeProduct(ctx context.Context, in *returnpb.VerifyExchangeProductRequest, opts ...grpcPkg.CallOption) (*returnpb.VerifyExchangeProductResponse, error) {
	m.lastVerifyExcReq = in
	status := "VERIFIED"
	if !in.IsApproved {
		status = "VERIFICATION_FAILED"
	}
	return &returnpb.VerifyExchangeProductResponse{
		Exchange: &returnpb.Exchange{
			Id:                in.ExchangeId,
			OrderId:           "order-1",
			OrderItemId:       "item-1",
			UserId:            "user-123",
			NewProductId:      "prod-2",
			OldBrand:          "Apple",
			OldModel:          "iPhone 13",
			BaseExchangeValue: 300.0,
			EstimatedDiscount: 250.0,
			FinalDiscount:     in.GetAdjustedDiscount(),
			Status:            status,
			Verification: &returnpb.ExchangeVerification{
				Id:               "ver-exc-1",
				ExchangeId:       in.ExchangeId,
				VerifiedBy:       in.VerifiedBy,
				ConditionRating:  in.ConditionRating,
				IsApproved:       in.IsApproved,
				ScreenIntact:     in.ScreenIntact,
				BodyIntact:       in.BodyIntact,
				Functional:       in.Functional,
				SerialMatches:    in.SerialMatches,
				AdjustedDiscount: in.GetAdjustedDiscount(),
				ProofImages:      in.ProofImages,
				Notes:            in.Notes,
				VerifiedAt:       "2026-10-26T12:00:00Z",
			},
		},
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

func TestRequestReturn_SingleItemID_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})
	itemID := "item-1"
	input := model.RequestReturnInput{
		OrderID: "order-456",
		ItemID:  &itemID,
		Reason:  "Defective",
	}

	res, err := resolver.Mutation().RequestReturn(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.OrderItemID == nil || *res.OrderItemID != "item-1" {
		t.Errorf("expected order item id 'item-1'")
	}
}

func TestRequestReturn_ItemIDsMismatch_BadRequest(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})
	orderItemID := "item-1"
	itemID := "item-2"
	input := model.RequestReturnInput{
		OrderID:     "order-456",
		OrderItemID: &orderItemID,
		ItemID:      &itemID,
		Reason:      "Defective",
	}

	_, err := resolver.Mutation().RequestReturn(ctx, input)
	if err == nil {
		t.Fatal("expected bad request error for mismatched item IDs, got nil")
	}
}

func TestRequestReturn_MissingOrderItemID_BadRequest(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})
	input := model.RequestReturnInput{
		OrderID: "order-456",
		Reason:  "Defective",
	}

	_, err := resolver.Mutation().RequestReturn(ctx, input)
	if err == nil {
		t.Fatal("expected bad request error for missing order item ID, got nil")
	}
}

func TestRequestReplacement_ItemIDsMismatch_BadRequest(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})
	orderItemID := "item-1"
	itemID := "item-2"
	input := model.RequestReplacementInput{
		OrderID:     "order-456",
		OrderItemID: &orderItemID,
		ItemID:      &itemID,
		Reason:      "Defective",
	}

	_, err := resolver.Mutation().RequestReplacement(ctx, input)
	if err == nil {
		t.Fatal("expected bad request error for mismatched item IDs, got nil")
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

	res, err := resolver.Mutation().UpdateReturnStatus(ctx, "ret-100", "APPROVED", nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Status != "APPROVED" {
		t.Errorf("expected status 'APPROVED', got '%s'", res.Status)
	}
}

func TestApproveReturn_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	res, err := resolver.Mutation().ApproveReturn(ctx, "ret-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "APPROVED" {
		t.Errorf("expected status 'APPROVED', got '%s'", res.Status)
	}
}

func TestApproveReturn_CustomerCannotApproveOwnReturn(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	// User user-1 is the owner of ret-100
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-1",
		Role:   "ADMIN",
	})

	_, err := resolver.Mutation().ApproveReturn(ctx, "ret-100")
	if err == nil {
		t.Fatal("expected error when customer tries to approve own return, got nil")
	}
}

func TestRejectReturn_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	res, err := resolver.Mutation().RejectReturn(ctx, "ret-100", "Damaged by customer")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Status != "REJECTED" {
		t.Errorf("expected status 'REJECTED', got '%s'", res.Status)
	}
	if res.RejectionReason == nil || *res.RejectionReason != "Damaged by customer" {
		t.Errorf("expected rejection reason 'Damaged by customer'")
	}
}

func TestRejectReturn_RequiresReason(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	_, err := resolver.Mutation().RejectReturn(ctx, "ret-100", "   ")
	if err == nil {
		t.Fatal("expected error when rejection reason is empty, got nil")
	}
}

func TestReturnOrder_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-1",
		Role:   "USER",
	})
	res, err := resolver.Query().ReturnOrder(ctx, "ret-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "ret-100" {
		t.Errorf("expected ID 'ret-100', got '%s'", res.ID)
	}
}

func TestReturnOrder_Unauthenticated(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	_, err := resolver.Query().ReturnOrder(context.Background(), "ret-100")
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

func TestReturnOrder_NonOwner_Forbidden(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-other",
		Role:   "USER",
	})
	_, err := resolver.Query().ReturnOrder(ctx, "ret-100")
	if err == nil {
		t.Fatal("expected forbidden error for non-owner, got nil")
	}
	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN error, got: %v", err)
	}
}

func TestReturnOrder_Admin_Bypass(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})
	res, err := resolver.Query().ReturnOrder(ctx, "ret-100")
	if err != nil {
		t.Fatalf("admin should bypass ownership check, got error: %v", err)
	}
	if res.ID != "ret-100" {
		t.Errorf("expected ID 'ret-100', got '%s'", res.ID)
	}
}

func TestReturnOrdersByOrder_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-1",
		Role:   "USER",
	})
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

func TestReturnOrdersByOrder_Unauthenticated(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	_, err := resolver.Query().ReturnOrdersByOrder(context.Background(), "order-123")
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

func TestReturnOrdersByOrder_NonOwner_Filtered(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-other",
		Role:   "USER",
	})
	res, err := resolver.Query().ReturnOrdersByOrder(ctx, "order-123")
	if err != nil {
		t.Fatalf("expected no error (filtered result), got: %v", err)
	}
	if len(res) != 0 {
		t.Errorf("expected 0 returns for non-owner, got %d", len(res))
	}
}

func TestReturnOrdersByOrder_Admin_SeesAll(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})
	res, err := resolver.Query().ReturnOrdersByOrder(ctx, "order-123")
	if err != nil {
		t.Fatalf("admin should see all returns, got error: %v", err)
	}
	if len(res) != 1 {
		t.Errorf("expected 1 return for admin, got %d", len(res))
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

func TestCreateExchange_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	ded := 50.0
	input := model.CreateExchangeInput{
		OrderID:           "order-1",
		OrderItemID:       "item-1",
		NewProductID:      "prod-2",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 300.0,
		Answers: []*model.ExchangeQuestionAnswerInput{
			{
				QuestionKey:     "screen_condition",
				AnswerValue:     "minor_scratches",
				DeductionAmount: &ded,
			},
		},
	}

	exc, err := resolver.Mutation().CreateExchange(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exc.ID != "exc-100" {
		t.Errorf("expected exchange ID 'exc-100', got '%s'", exc.ID)
	}
	if mockClient.lastCreateExcReq.UserId != "user-123" {
		t.Errorf("expected user ID 'user-123', got '%s'", mockClient.lastCreateExcReq.UserId)
	}
	if len(mockClient.lastCreateExcReq.Answers) != 1 {
		t.Fatalf("expected 1 answer, got %d", len(mockClient.lastCreateExcReq.Answers))
	}
	if mockClient.lastCreateExcReq.Answers[0].QuestionKey != "screen_condition" {
		t.Errorf("expected question key 'screen_condition', got '%s'", mockClient.lastCreateExcReq.Answers[0].QuestionKey)
	}
}

func TestCreateExchange_Unauthorized(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	input := model.CreateExchangeInput{
		OrderID:           "order-1",
		OrderItemID:       "item-1",
		NewProductID:      "prod-2",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 300.0,
	}

	_, err := resolver.Mutation().CreateExchange(context.Background(), input)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}
}

func TestCreateExchange_WithInputUserID_Success(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	targetUser := "user-custom-456"
	input := model.CreateExchangeInput{
		OrderID:           "order-1",
		UserID:            &targetUser,
		OrderItemID:       "item-1",
		NewProductID:      "prod-2",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 300.0,
	}

	exc, err := resolver.Mutation().CreateExchange(context.Background(), input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exc.ID != "exc-100" {
		t.Errorf("expected exc-100, got %s", exc.ID)
	}
	if mockClient.lastCreateExcReq.UserId != "user-custom-456" {
		t.Errorf("expected user ID 'user-custom-456', got '%s'", mockClient.lastCreateExcReq.UserId)
	}
}

func TestCreateExchange_Forbidden_OtherUser(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	otherUser := "user-999"
	input := model.CreateExchangeInput{
		OrderID:           "order-1",
		UserID:            &otherUser,
		OrderItemID:       "item-1",
		NewProductID:      "prod-2",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 300.0,
	}

	_, err := resolver.Mutation().CreateExchange(ctx, input)
	if err == nil {
		t.Fatal("expected forbidden error, got nil")
	}
}

func TestCreateExchange_Admin_OverrideCustomer(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-1",
		Role:   "ADMIN",
	})

	customerUser := "customer-100"
	input := model.CreateExchangeInput{
		OrderID:           "order-1",
		UserID:            &customerUser,
		OrderItemID:       "item-1",
		NewProductID:      "prod-2",
		OldBrand:          "Apple",
		OldModel:          "iPhone 12",
		BaseExchangeValue: 300.0,
	}

	exc, err := resolver.Mutation().CreateExchange(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exc == nil {
		t.Fatal("expected exchange, got nil")
	}
	if mockClient.lastCreateExcReq.UserId != "customer-100" {
		t.Errorf("expected user ID 'customer-100', got '%s'", mockClient.lastCreateExcReq.UserId)
	}
}

func TestVerifyExchangeProduct_Success_DeliveryPartner(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "agent-1",
		PartnerID: "partner-1",
		Role:      "DELIVERY_PARTNER",
	})

	adj := 280.0
	notes := "Inspected at doorstep, minor scratches verified"
	input := model.VerifyExchangeProductInput{
		ExchangeID:       "exc-100",
		ConditionRating:  "GOOD",
		IsApproved:       true,
		ScreenIntact:     true,
		BodyIntact:       true,
		Functional:       true,
		SerialMatches:    true,
		AdjustedDiscount: &adj,
		Notes:            &notes,
	}

	exc, err := resolver.Mutation().VerifyExchangeProduct(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if exc.Status != "VERIFIED" {
		t.Errorf("expected status 'VERIFIED', got '%s'", exc.Status)
	}
	if exc.Verification == nil {
		t.Fatal("expected verification details, got nil")
	}
	if exc.Verification.VerifiedBy != "partner-1" {
		t.Errorf("expected verifiedBy 'partner-1', got '%s'", exc.Verification.VerifiedBy)
	}
}

func TestVerifyExchangeProduct_Forbidden_Customer(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	input := model.VerifyExchangeProductInput{
		ExchangeID:      "exc-100",
		ConditionRating: "EXCELLENT",
		IsApproved:      true,
	}

	_, err := resolver.Mutation().VerifyExchangeProduct(ctx, input)
	if err == nil {
		t.Fatal("expected forbidden error for regular user, got nil")
	}
}

func TestVerifyExchangeProduct_InvalidConditionRating(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "agent-1",
		PartnerID: "partner-1",
		Role:      "DELIVERY_PARTNER",
	})

	input := model.VerifyExchangeProductInput{
		ExchangeID:      "exc-100",
		ConditionRating: "NOT_A_VALID_RATING",
		IsApproved:      true,
	}

	_, err := resolver.Mutation().VerifyExchangeProduct(ctx, input)
	if err == nil {
		t.Fatal("expected bad request error for invalid condition rating, got nil")
	}
}

func TestExchangeQuery_Success_Owner(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "USER",
	})

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exc.ID != "exc-100" {
		t.Errorf("expected exchange ID 'exc-100', got '%s'", exc.ID)
	}
}

func TestExchangeQuery_Forbidden_OtherUser(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "other-user",
		Role:   "USER",
	})

	_, err := resolver.Query().Exchange(ctx, "exc-100")
	if err == nil {
		t.Fatal("expected forbidden error for other user, got nil")
	}
}

type mockOrderClientForReturnTest struct {
	orderpb.OrderServiceClient
	order *orderpb.Order
	err   error
}

func (m *mockOrderClientForReturnTest) GetOrder(ctx context.Context, in *orderpb.GetOrderRequest, opts ...grpcPkg.CallOption) (*orderpb.GetOrderResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.order != nil && m.order.Id == in.Id {
		return &orderpb.GetOrderResponse{Order: m.order}, nil
	}
	return nil, appErrors.NotFound("order not found")
}

type mockDeliveryClientForReturnTest struct {
	deliverypb.DeliveryServiceClient
	delivery *deliverypb.Delivery
	err      error
}

func (m *mockDeliveryClientForReturnTest) GetDeliveryByOrder(ctx context.Context, in *deliverypb.GetDeliveryByOrderRequest, opts ...grpcPkg.CallOption) (*deliverypb.GetDeliveryResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.delivery != nil && m.delivery.OrderId == in.OrderId {
		return &deliverypb.GetDeliveryResponse{Delivery: m.delivery}, nil
	}
	return nil, appErrors.NotFound("delivery not found")
}

type mockStoreClientForReturnTest struct {
	storepb.StoreServiceClient
	store *storepb.Store
	err   error
}

func (m *mockStoreClientForReturnTest) GetStore(ctx context.Context, in *storepb.GetStoreRequest, opts ...grpcPkg.CallOption) (*storepb.GetStoreResponse, error) {
	if m.err != nil {
		return nil, m.err
	}
	if m.store != nil && (m.store.Id == in.Id || (in.MerchantId != "" && m.store.MerchantId == in.MerchantId)) {
		return &storepb.GetStoreResponse{Store: m.store}, nil
	}
	return nil, appErrors.NotFound("store not found")
}

func TestExchangeQuery_Admin_Bypass(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{ReturnClient: mockClient}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-999",
		Role:   "ADMIN",
	})

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error for admin: %v", err)
	}
	if exc == nil || exc.ID != "exc-100" {
		t.Fatalf("expected exchange exc-100, got %v", exc)
	}
}

func TestExchangeQuery_Merchant_Forbidden_UnauthorizedOrder(t *testing.T) {
	mockClient := &mockReturnClient{}
	orderMock := &mockOrderClientForReturnTest{
		order: &orderpb.Order{
			Id:      "order-1",
			UserId:  "customer-1",
			StoreId: "store-other",
		},
	}
	storeMock := &mockStoreClientForReturnTest{
		store: &storepb.Store{
			Id:         "store-other",
			MerchantId: "merchant-other",
		},
	}
	clients := &grpc.Clients{
		ReturnClient: mockClient,
		OrderClient:  orderMock,
		StoreClient:  storeMock,
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:     "merchant-1",
		MerchantID: "merchant-1",
		Role:       "MERCHANT",
	})

	_, err := resolver.Query().Exchange(ctx, "exc-100")
	if err == nil {
		t.Fatal("expected forbidden error for unauthorized merchant, got nil")
	}
}

func TestExchangeQuery_Merchant_Success_OwnStoreOrder(t *testing.T) {
	mockClient := &mockReturnClient{}
	orderMock := &mockOrderClientForReturnTest{
		order: &orderpb.Order{
			Id:      "order-1",
			UserId:  "customer-1",
			StoreId: "store-100",
		},
	}
	storeMock := &mockStoreClientForReturnTest{
		store: &storepb.Store{
			Id:         "store-100",
			MerchantId: "merchant-1",
		},
	}
	clients := &grpc.Clients{
		ReturnClient: mockClient,
		OrderClient:  orderMock,
		StoreClient:  storeMock,
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:     "merchant-1",
		MerchantID: "merchant-1",
		Role:       "MERCHANT",
	})

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error for authorized merchant: %v", err)
	}
	if exc == nil || exc.ID != "exc-100" {
		t.Fatalf("expected exchange exc-100, got %v", exc)
	}
}

func TestExchangeQuery_Merchant_Success_ItemStoreMatch(t *testing.T) {
	mockClient := &mockReturnClient{}
	orderMock := &mockOrderClientForReturnTest{
		order: &orderpb.Order{
			Id:      "order-1",
			UserId:  "customer-1",
			StoreId: "store-other",
			Items: []*orderpb.OrderItem{
				{
					Id:      "item-1",
					StoreId: "store-100",
				},
			},
		},
	}
	storeMock := &mockStoreClientForReturnTest{
		store: &storepb.Store{
			Id:         "store-100",
			MerchantId: "merchant-1",
		},
	}
	clients := &grpc.Clients{
		ReturnClient: mockClient,
		OrderClient:  orderMock,
		StoreClient:  storeMock,
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:     "merchant-1",
		MerchantID: "merchant-1",
		Role:       "MERCHANT",
	})

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error for authorized merchant via item store: %v", err)
	}
	if exc == nil || exc.ID != "exc-100" {
		t.Fatalf("expected exchange exc-100, got %v", exc)
	}
}

func TestExchangeQuery_DeliveryPartner_Forbidden_Unassigned(t *testing.T) {
	mockClient := &mockReturnClient{}
	delMock := &mockDeliveryClientForReturnTest{
		delivery: &deliverypb.Delivery{
			OrderId:         "order-1",
			DeliveryPartner: "partner-other",
		},
	}
	clients := &grpc.Clients{
		ReturnClient:   mockClient,
		DeliveryClient: delMock,
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "agent-1",
		PartnerID: "partner-1",
		Role:      "DELIVERY_PARTNER",
	})

	_, err := resolver.Query().Exchange(ctx, "exc-100")
	if err == nil {
		t.Fatal("expected forbidden error for unassigned delivery partner, got nil")
	}
}

func TestExchangeQuery_DeliveryPartner_Success_AssignedDelivery(t *testing.T) {
	mockClient := &mockReturnClient{}
	delMock := &mockDeliveryClientForReturnTest{
		delivery: &deliverypb.Delivery{
			OrderId:         "order-1",
			DeliveryPartner: "partner-1",
		},
	}
	clients := &grpc.Clients{
		ReturnClient:   mockClient,
		DeliveryClient: delMock,
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "agent-1",
		PartnerID: "partner-1",
		Role:      "DELIVERY_PARTNER",
	})

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error for assigned delivery partner: %v", err)
	}
	if exc == nil || exc.ID != "exc-100" {
		t.Fatalf("expected exchange exc-100, got %v", exc)
	}
}

func TestExchangeQuery_DeliveryPartner_Success_VerifiedBy(t *testing.T) {
	mockClient := &mockReturnClient{}
	clients := &grpc.Clients{
		ReturnClient: mockClient,
	}
	resolver := NewResolver(clients)

	// In mockReturnClient, GetExchange returns an exchange with ID exc-100.
	// If we set Verification.VerifiedBy to partner-1:
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID:    "agent-1",
		PartnerID: "partner-1",
		Role:      "DELIVERY_PARTNER",
	})

	// Wrap or test when Verification.VerifiedBy matches:
	originalGetExchange := mockClient.GetExchange
	_ = originalGetExchange
	// We can update mockClient to return an exchange verified by partner-1
	mockClientWithVerification := &mockReturnClientWithVerifiedBy{
		mockReturnClient: *mockClient,
		verifiedBy:       "partner-1",
	}
	clients.ReturnClient = mockClientWithVerification

	exc, err := resolver.Query().Exchange(ctx, "exc-100")
	if err != nil {
		t.Fatalf("unexpected error for verifying delivery partner: %v", err)
	}
	if exc == nil || exc.ID != "exc-100" {
		t.Fatalf("expected exchange exc-100, got %v", exc)
	}
}

type mockReturnClientWithVerifiedBy struct {
	mockReturnClient
	verifiedBy string
}

func (m *mockReturnClientWithVerifiedBy) GetExchange(ctx context.Context, in *returnpb.GetExchangeRequest, opts ...grpcPkg.CallOption) (*returnpb.GetExchangeResponse, error) {
	resp, err := m.mockReturnClient.GetExchange(ctx, in, opts...)
	if err != nil {
		return nil, err
	}
	if resp != nil && resp.Exchange != nil {
		resp.Exchange.Verification = &returnpb.ExchangeVerification{
			VerifiedBy: m.verifiedBy,
		}
	}
	return resp, nil
}


