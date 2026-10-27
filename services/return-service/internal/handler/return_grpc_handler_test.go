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
	updateReturnStatusFunc   func(ctx context.Context, id string, status model.ReturnStatus) (*model.Return, error)
	verifyProductFunc        func(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error)
	listReturnsFunc          func(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error)
	listReturnsByOrderIDFunc func(ctx context.Context, orderID string) ([]*model.Return, error)
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

func (m *mockReturnService) UpdateReturnStatus(ctx context.Context, id string, st model.ReturnStatus) (*model.Return, error) {
	if m.updateReturnStatusFunc != nil {
		return m.updateReturnStatusFunc(ctx, id, st)
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
		{"DuplicateReturn", service.ErrDuplicateReturnRequest, codes.AlreadyExists},
		{"OrderNotDelivered", service.ErrOrderNotDelivered, codes.FailedPrecondition},
		{"WindowExpired", service.ErrReturnWindowExpired, codes.FailedPrecondition},
		{"ProductNotReturnable", service.ErrProductNotReturnable, codes.FailedPrecondition},
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
		updateReturnStatusFunc: func(ctx context.Context, id string, st model.ReturnStatus) (*model.Return, error) {
			if !st.IsValid() {
				return nil, service.ErrInvalidReturnStatus
			}
			return &model.Return{
				ID:          id,
				OrderID:     "order-1",
				OrderItemID: "item-1",
				UserID:      "user-1",
				Status:      st,
				RequestedAt: now,
				CreatedAt:   now,
				UpdatedAt:   now,
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

