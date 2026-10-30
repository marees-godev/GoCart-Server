package handler_test

import (
	"context"
	"testing"
	"time"

	returns "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockReturnService struct {
	requestReturnFunc        func(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error)
	getReturnFunc            func(ctx context.Context, id string) (*model.Return, error)
	updateReturnStatusFunc   func(ctx context.Context, id string, status model.ReturnStatus, rejectionReason ...string) (*model.Return, error)
	approveReturnFunc        func(ctx context.Context, id string, reviewerID string) (*model.Return, error)
	rejectReturnFunc         func(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error)
	verifyProductFunc        func(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error)
	listReturnsFunc          func(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error)
	listReturnsByOrderIDFunc func(ctx context.Context, orderID string) ([]*model.Return, error)
	createExchangeFunc        func(ctx context.Context, req *dto.CreateExchangeDTO) (*model.Exchange, error)
	getExchangeFunc           func(ctx context.Context, id string) (*model.Exchange, error)
	verifyExchangeProductFunc func(ctx context.Context, req *dto.VerifyExchangeProductDTO) (*model.Exchange, *model.ExchangeVerification, error)
}

func (m *mockReturnService) RequestReturn(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error) {
	if m.requestReturnFunc != nil {
		return m.requestReturnFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockReturnService) GetReturn(ctx context.Context, id string) (*model.Return, error) {
	if m.getReturnFunc != nil {
		return m.getReturnFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockReturnService) UpdateReturnStatus(ctx context.Context, id string, st model.ReturnStatus, rejectionReason ...string) (*model.Return, error) {
	if m.updateReturnStatusFunc != nil {
		return m.updateReturnStatusFunc(ctx, id, st, rejectionReason...)
	}
	return nil, nil
}

func (m *mockReturnService) ApproveReturn(ctx context.Context, id string, reviewerID string) (*model.Return, error) {
	if m.approveReturnFunc != nil {
		return m.approveReturnFunc(ctx, id, reviewerID)
	}
	return nil, nil
}

func (m *mockReturnService) RejectReturn(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error) {
	if m.rejectReturnFunc != nil {
		return m.rejectReturnFunc(ctx, id, rejectionReason, reviewerID)
	}
	return nil, nil
}

func (m *mockReturnService) VerifyProduct(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error) {
	if m.verifyProductFunc != nil {
		return m.verifyProductFunc(ctx, req)
	}
	return nil, nil, nil
}

func (m *mockReturnService) ListReturns(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error) {
	if m.listReturnsFunc != nil {
		return m.listReturnsFunc(ctx, req)
	}
	return nil, 0, nil
}

func (m *mockReturnService) ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error) {
	if m.listReturnsByOrderIDFunc != nil {
		return m.listReturnsByOrderIDFunc(ctx, orderID)
	}
	return nil, nil
}

func (m *mockReturnService) CreateExchange(ctx context.Context, req *dto.CreateExchangeDTO) (*model.Exchange, error) {
	if m.createExchangeFunc != nil {
		return m.createExchangeFunc(ctx, req)
	}
	return nil, nil
}

func (m *mockReturnService) GetExchange(ctx context.Context, id string) (*model.Exchange, error) {
	if m.getExchangeFunc != nil {
		return m.getExchangeFunc(ctx, id)
	}
	return nil, nil
}

func (m *mockReturnService) VerifyExchangeProduct(ctx context.Context, req *dto.VerifyExchangeProductDTO) (*model.Exchange, *model.ExchangeVerification, error) {
	if m.verifyExchangeProductFunc != nil {
		return m.verifyExchangeProductFunc(ctx, req)
	}
	return nil, nil, nil
}


func TestReturnGRPCHandler_RequestReturn_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		requestReturnFunc: func(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error) {
			return &model.Return{
				ID:           "ret-101",
				OrderID:      req.OrderID,
				OrderItemID:  req.OrderItemID,
				UserID:       req.UserID,
				Reason:       req.Reason,
				Status:       model.ReturnStatusRequested,
				RefundAmount: 99.99,
				RequestedAt:  now,
				CreatedAt:    now,
				UpdatedAt:    now,
			}, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	res, err := h.RequestReturn(ctx, &returns.RequestReturnRequest{
		OrderId:     "order-1",
		OrderItemId: "item-1",
		UserId:      "user-1",
		Reason:      "Wrong color",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ReturnOrder.Id != "ret-101" {
		t.Errorf("expected return id ret-101, got %s", res.ReturnOrder.Id)
	}
	if res.ReturnOrder.RefundAmount != 99.99 {
		t.Errorf("expected refund amount 99.99, got %f", res.ReturnOrder.RefundAmount)
	}
	if res.ReturnOrder.Status != string(model.ReturnStatusRequested) {
		t.Errorf("expected status %s, got %s", model.ReturnStatusRequested, res.ReturnOrder.Status)
	}
}

func TestReturnGRPCHandler_RequestReturn_ErrorMapping(t *testing.T) {
	ctx := context.Background()

	testCases := []struct {
		name         string
		serviceErr   error
		expectedCode codes.Code
	}{
		{"ReasonMandatory", service.ErrReturnReasonRequired, codes.InvalidArgument},
		{"OrderItemRequired", service.ErrOrderItemRequired, codes.InvalidArgument},
		{"MultipleItemsNotSupported", service.ErrMultipleItemsNotSupported, codes.InvalidArgument},
		{"OrderItemMismatch", service.ErrOrderItemMismatch, codes.InvalidArgument},
		{"DuplicateReturn", service.ErrDuplicateReturnRequest, codes.AlreadyExists},
		{"OrderNotDelivered", service.ErrOrderNotDelivered, codes.FailedPrecondition},
		{"WindowExpired", service.ErrReturnWindowExpired, codes.FailedPrecondition},
		{"ProductNotReturnable", service.ErrProductNotReturnable, codes.FailedPrecondition},
		{"InvalidStateTransition", service.ErrInvalidStateTransition, codes.FailedPrecondition},
		{"ItemNotFound", service.ErrOrderItemNotFound, codes.NotFound},
		{"OrderNotFound", service.ErrOrderNotFound, codes.NotFound},
		{"UnauthorizedUser", service.ErrUnauthorizedReturnRequest, codes.PermissionDenied},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			mockSvc := &mockReturnService{
				requestReturnFunc: func(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error) {
					return nil, tc.serviceErr
				},
			}
			h := handler.NewReturnGRPCHandler(mockSvc, nil)

			_, err := h.RequestReturn(ctx, &returns.RequestReturnRequest{
				OrderId:     "order-1",
				OrderItemId: "item-1",
				UserId:      "user-1",
				Reason:      "reason",
			})
			if err == nil {
				t.Fatalf("expected error, got nil")
			}

			st, ok := status.FromError(err)
			if !ok {
				t.Fatalf("expected gRPC status error, got: %v", err)
			}
			if st.Code() != tc.expectedCode {
				t.Errorf("expected code %v, got %v (message: %s)", tc.expectedCode, st.Code(), st.Message())
			}
		})
	}
}

func TestReturnGRPCHandler_GetReturn_SuccessAndNotFound(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		getReturnFunc: func(ctx context.Context, id string) (*model.Return, error) {
			if id == "ret-exists" {
				return &model.Return{
					ID:          "ret-exists",
					OrderID:     "order-1",
					OrderItemID: "item-1",
					UserID:      "user-1",
					Reason:      "Damaged",
					Status:      model.ReturnStatusRequested,
					RequestedAt: now,
					CreatedAt:   now,
					UpdatedAt:   now,
				}, nil
			}
			return nil, repository.ErrReturnNotFound
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	// Success
	res, err := h.GetReturn(ctx, &returns.GetReturnRequest{Id: "ret-exists"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ReturnOrder.Id != "ret-exists" {
		t.Errorf("expected ID ret-exists, got %s", res.ReturnOrder.Id)
	}

	// Not found
	_, err = h.GetReturn(ctx, &returns.GetReturnRequest{Id: "ret-missing"})
	if err == nil {
		t.Fatalf("expected error for missing return")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.NotFound {
		t.Errorf("expected NotFound code, got: %v", err)
	}
}

func TestReturnGRPCHandler_UpdateReturnStatus_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		updateReturnStatusFunc: func(ctx context.Context, id string, st model.ReturnStatus, rejectionReason ...string) (*model.Return, error) {
			if !st.IsValid() {
				return nil, service.ErrInvalidReturnStatus
			}
			var rejReason string
			if len(rejectionReason) > 0 {
				rejReason = rejectionReason[0]
			}
			return &model.Return{
				ID:              id,
				OrderID:         "order-1",
				OrderItemID:     "item-1",
				UserID:          "user-1",
				Status:          st,
				RejectionReason: rejReason,
				RequestedAt:     now,
				CreatedAt:       now,
				UpdatedAt:       now,
			}, nil
		},
		approveReturnFunc: func(ctx context.Context, id string, reviewerID string) (*model.Return, error) {
			return &model.Return{
				ID:          id,
				OrderID:     "order-1",
				OrderItemID: "item-1",
				UserID:      "user-1",
				Status:      model.ReturnStatusApproved,
				RequestedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}, nil
		},
		rejectReturnFunc: func(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error) {
			return &model.Return{
				ID:              id,
				OrderID:         "order-1",
				OrderItemID:     "item-1",
				UserID:          "user-1",
				Status:          model.ReturnStatusRejected,
				RejectionReason: rejectionReason,
				RequestedAt:     now,
				CreatedAt:       now,
				UpdatedAt:       now,
			}, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	res, err := h.UpdateReturnStatus(ctx, &returns.UpdateReturnStatusRequest{
		Id:     "ret-1",
		Status: string(model.ReturnStatusApproved),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ReturnOrder.Status != string(model.ReturnStatusApproved) {
		t.Errorf("expected status %s, got %s", model.ReturnStatusApproved, res.ReturnOrder.Status)
	}

	// ApproveReturn
	apprRes, err := h.ApproveReturn(ctx, &returns.ApproveReturnRequest{
		Id:         "ret-1",
		ReviewerId: "admin-1",
	})
	if err != nil {
		t.Fatalf("unexpected error on ApproveReturn: %v", err)
	}
	if apprRes.ReturnOrder.Status != string(model.ReturnStatusApproved) {
		t.Errorf("expected status %s, got %s", model.ReturnStatusApproved, apprRes.ReturnOrder.Status)
	}

	// RejectReturn
	rejRes, err := h.RejectReturn(ctx, &returns.RejectReturnRequest{
		Id:              "ret-1",
		RejectionReason: "Policy violation",
		ReviewerId:      "admin-1",
	})
	if err != nil {
		t.Fatalf("unexpected error on RejectReturn: %v", err)
	}
	if rejRes.ReturnOrder.Status != string(model.ReturnStatusRejected) {
		t.Errorf("expected status %s, got %s", model.ReturnStatusRejected, rejRes.ReturnOrder.Status)
	}
	if rejRes.ReturnOrder.RejectionReason != "Policy violation" {
		t.Errorf("expected rejection reason 'Policy violation', got %s", rejRes.ReturnOrder.RejectionReason)
	}

	// Invalid status
	_, err = h.UpdateReturnStatus(ctx, &returns.UpdateReturnStatusRequest{
		Id:     "ret-1",
		Status: "INVALID",
	})
	if err == nil {
		t.Fatalf("expected error for invalid status")
	}
	st, ok := status.FromError(err)
	if !ok || st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got: %v", err)
	}
}

func TestReturnGRPCHandler_VerifyProduct_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		verifyProductFunc: func(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error) {
			ret := &model.Return{
				ID:          req.ReturnID,
				OrderID:     "order-1",
				OrderItemID: "item-1",
				UserID:      "user-1",
				Status:      model.ReturnStatusVerified,
				RequestedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			v := &model.ProductVerification{
				ID:          "ver-1",
				ReturnID:    req.ReturnID,
				VerifiedBy:  req.VerifiedBy,
				Condition:   model.VerificationCondition(req.Condition),
				IsApproved:  req.IsApproved,
				ProofImages: req.ProofImages,
				Notes:       req.Notes,
				VerifiedAt:  now,
				CreatedAt:   now,
				UpdatedAt:   now,
			}
			ret.Verification = v
			return ret, v, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	res, err := h.VerifyProduct(ctx, &returns.VerifyProductRequest{
		ReturnId:    "ret-1",
		VerifiedBy:  "partner-1",
		Condition:   "GOOD",
		IsApproved:  true,
		ProofImages: []string{"https://example.com/proof1.jpg"},
		Notes:       "condition verified",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.ReturnOrder == nil || res.ReturnOrder.Status != string(model.ReturnStatusVerified) {
		t.Fatalf("expected ReturnOrder status VERIFIED, got: %v", res.ReturnOrder)
	}
	if res.Verification == nil || len(res.Verification.ProofImages) != 1 {
		t.Fatalf("expected 1 proof image, got: %v", res.Verification)
	}
}

func TestReturnGRPCHandler_ListReturns_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		listReturnsFunc: func(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error) {
			return []*model.Return{
				{
					ID:          "ret-1",
					OrderID:     "order-1",
					OrderItemID: "item-1",
					UserID:      "user-1",
					Status:      model.ReturnStatusRequested,
					Type:        model.ReturnTypeReturn,
					CreatedAt:   now,
					UpdatedAt:   now,
					RequestedAt: now,
				},
			}, 1, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)
	res, err := h.ListReturns(ctx, &returns.ListReturnsRequest{Limit: 10, Offset: 0})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Total != 1 || len(res.Returns) != 1 {
		t.Fatalf("expected 1 return, got: %v", res)
	}
}

func TestReturnGRPCHandler_ApproveReturn_SuccessAndPermissionDenied(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		approveReturnFunc: func(ctx context.Context, id string, reviewerID string) (*model.Return, error) {
			if reviewerID == "customer-1" {
				return nil, service.ErrCustomerCannotReviewOwnReturn
			}
			return &model.Return{
				ID:          id,
				OrderID:     "order-1",
				OrderItemID: "item-1",
				UserID:      "customer-1",
				Status:      model.ReturnStatusApproved,
				Type:        model.ReturnTypeReturn,
				CreatedAt:   now,
				UpdatedAt:   now,
				RequestedAt: now,
			}, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	// 1. Success as admin
	res, err := h.ApproveReturn(ctx, &returns.ApproveReturnRequest{Id: "ret-1", ReviewerId: "admin-1"})
	if err != nil {
		t.Fatalf("unexpected error approving return: %v", err)
	}
	if res.ReturnOrder.Status != string(model.ReturnStatusApproved) {
		t.Errorf("expected APPROVED status, got %s", res.ReturnOrder.Status)
	}

	// 2. PermissionDenied when customer attempts to approve own return
	_, err = h.ApproveReturn(ctx, &returns.ApproveReturnRequest{Id: "ret-1", ReviewerId: "customer-1"})
	if status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", err)
	}
}

func TestReturnGRPCHandler_RejectReturn_SuccessAndReasonRequired(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()

	mockSvc := &mockReturnService{
		rejectReturnFunc: func(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error) {
			if rejectionReason == "" {
				return nil, service.ErrRejectionReasonRequired
			}
			return &model.Return{
				ID:              id,
				OrderID:         "order-1",
				OrderItemID:     "item-1",
				UserID:          "customer-1",
				Status:          model.ReturnStatusRejected,
				RejectionReason: rejectionReason,
				Type:            model.ReturnTypeReturn,
				CreatedAt:       now,
				UpdatedAt:       now,
				RequestedAt:     now,
			}, nil
		},
	}

	h := handler.NewReturnGRPCHandler(mockSvc, nil)

	// 1. Success with reason
	res, err := h.RejectReturn(ctx, &returns.RejectReturnRequest{
		Id:              "ret-1",
		RejectionReason: "Policy non-compliance",
		ReviewerId:      "admin-1",
	})
	if err != nil {
		t.Fatalf("unexpected error rejecting return: %v", err)
	}
	if res.ReturnOrder.Status != string(model.ReturnStatusRejected) {
		t.Errorf("expected REJECTED status, got %s", res.ReturnOrder.Status)
	}
	if res.ReturnOrder.RejectionReason != "Policy non-compliance" {
		t.Errorf("expected rejection reason, got %s", res.ReturnOrder.RejectionReason)
	}

	// 2. InvalidArgument when reason required
	_, err = h.RejectReturn(ctx, &returns.RejectReturnRequest{
		Id:              "ret-1",
		RejectionReason: "",
		ReviewerId:      "admin-1",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument, got %v", err)
	}
}

func TestReturnGRPCHandler_RequestReturn_ItemIDMismatch_Rejected(t *testing.T) {
	ctx := context.Background()
	h := handler.NewReturnGRPCHandler(&mockReturnService{}, nil)
	_, err := h.RequestReturn(ctx, &returns.RequestReturnRequest{
		OrderId:     "order-1",
		OrderItemId: "item-1",
		ItemId:      "item-2",
		UserId:      "user-1",
		Reason:      "damaged",
	})
	if status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for mismatched item ID, got: %v", err)
	}
}

func TestReturnGRPCHandler_RequestReturn_SingleItemID_Success(t *testing.T) {
	ctx := context.Background()
	var capturedOrderItemID string
	svc := &mockReturnService{
		requestReturnFunc: func(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error) {
			capturedOrderItemID = req.OrderItemID
			return &model.Return{ID: "ret-1", OrderItemID: req.OrderItemID}, nil
		},
	}
	h := handler.NewReturnGRPCHandler(svc, nil)
	res, err := h.RequestReturn(ctx, &returns.RequestReturnRequest{
		OrderId: "order-1",
		ItemId:  "item-1",
		UserId:  "user-1",
		Reason:  "damaged",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedOrderItemID != "item-1" {
		t.Errorf("expected captured order item id 'item-1', got '%s'", capturedOrderItemID)
	}
	if res.ReturnOrder.OrderItemId != "item-1" {
		t.Errorf("expected return order item id 'item-1', got '%s'", res.ReturnOrder.OrderItemId)
	}
}

func TestReturnGRPCHandler_CreateExchange_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc := &mockReturnService{
		createExchangeFunc: func(ctx context.Context, req *dto.CreateExchangeDTO) (*model.Exchange, error) {
			return &model.Exchange{
				ID:                "exc-101",
				OrderID:           req.OrderID,
				UserID:            req.UserID,
				NewProductID:      req.NewProductID,
				OldBrand:          req.OldBrand,
				OldModel:          req.OldModel,
				BaseExchangeValue: req.BaseExchangeValue,
				EstimatedDiscount: 150.0,
				Status:            model.ExchangeStatusOfferCreated,
				CreatedAt:         now,
				UpdatedAt:         now,
			}, nil
		},
	}
	h := handler.NewReturnGRPCHandler(svc, nil)
	resp, err := h.CreateExchange(ctx, &returns.CreateExchangeRequest{
		OrderId:           "order-101",
		UserId:            "user-101",
		NewProductId:      "prod-101",
		OldBrand:          "Apple",
		OldModel:          "iPhone 11",
		BaseExchangeValue: 200.0,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Exchange.Id != "exc-101" {
		t.Errorf("expected exc-101, got: %s", resp.Exchange.Id)
	}
	if resp.Exchange.EstimatedDiscount != 150.0 {
		t.Errorf("expected discount 150, got: %f", resp.Exchange.EstimatedDiscount)
	}
}

func TestReturnGRPCHandler_VerifyExchangeProduct_Success(t *testing.T) {
	ctx := context.Background()
	now := time.Now().UTC()
	svc := &mockReturnService{
		verifyExchangeProductFunc: func(ctx context.Context, req *dto.VerifyExchangeProductDTO) (*model.Exchange, *model.ExchangeVerification, error) {
			finalDiscount := 0.0
			if req.AdjustedDiscount != nil {
				finalDiscount = *req.AdjustedDiscount
			}
			return &model.Exchange{
					ID:            req.ExchangeID,
					FinalDiscount: finalDiscount,
					Status:        model.ExchangeStatusVerified,
					CreatedAt:     now,
					UpdatedAt:     now,
				}, &model.ExchangeVerification{
					ID:               "ver-202",
					ExchangeID:       req.ExchangeID,
					VerifiedBy:       req.VerifiedBy,
					ConditionRating:  model.RatingGood,
					IsApproved:       req.IsApproved,
					ScreenIntact:     req.ScreenIntact,
					Functional:       req.Functional,
					SerialMatches:    req.SerialMatches,
					AdjustedDiscount: finalDiscount,
					VerifiedAt:       now,
					CreatedAt:        now,
					UpdatedAt:        now,
				}, nil
		},
	}
	h := handler.NewReturnGRPCHandler(svc, nil)
	adjDiscount := 150.0
	resp, err := h.VerifyExchangeProduct(ctx, &returns.VerifyExchangeProductRequest{
		ExchangeId:       "exc-101",
		VerifiedBy:       "delivery-1",
		ConditionRating:  "GOOD",
		IsApproved:       true,
		ScreenIntact:     true,
		BodyIntact:       true,
		Functional:       true,
		SerialMatches:    true,
		AdjustedDiscount: &adjDiscount,
		Notes:            "All good",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Exchange.Status != "VERIFIED" {
		t.Errorf("expected VERIFIED, got: %s", resp.Exchange.Status)
	}
	if !resp.Verification.IsApproved {
		t.Errorf("expected verification to be approved")
	}
}


