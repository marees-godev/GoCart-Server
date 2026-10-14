package resolvers

import (
	"context"
	"testing"

	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockPaymentClientForResolvers struct {
	paymentpb.PaymentServiceClient
	getPaymentResp      *paymentpb.GetPaymentResponse
	getPaymentErr       error
	getByOrderIDResp    *paymentpb.GetPaymentResponse
	getByOrderIDErr     error
	cancelPaymentResp   *paymentpb.CancelPaymentResponse
	cancelPaymentErr    error
	lastGetReq          *paymentpb.GetPaymentRequest
	lastGetByOrderIDReq *paymentpb.GetPaymentByOrderIDRequest
	lastCancelReq       *paymentpb.CancelPaymentRequest
}

func (m *mockPaymentClientForResolvers) GetPayment(ctx context.Context, in *paymentpb.GetPaymentRequest, opts ...grpcPkg.CallOption) (*paymentpb.GetPaymentResponse, error) {
	m.lastGetReq = in
	if m.getPaymentErr != nil {
		return nil, m.getPaymentErr
	}
	return m.getPaymentResp, nil
}

func (m *mockPaymentClientForResolvers) GetPaymentByOrderID(ctx context.Context, in *paymentpb.GetPaymentByOrderIDRequest, opts ...grpcPkg.CallOption) (*paymentpb.GetPaymentResponse, error) {
	m.lastGetByOrderIDReq = in
	if m.getByOrderIDErr != nil {
		return nil, m.getByOrderIDErr
	}
	return m.getByOrderIDResp, nil
}

func (m *mockPaymentClientForResolvers) CancelPayment(ctx context.Context, in *paymentpb.CancelPaymentRequest, opts ...grpcPkg.CallOption) (*paymentpb.CancelPaymentResponse, error) {
	m.lastCancelReq = in
	if m.cancelPaymentErr != nil {
		return nil, m.cancelPaymentErr
	}
	return m.cancelPaymentResp, nil
}

func newTestPaymentResolver(mock *mockPaymentClientForResolvers) *mutationResolver {
	clientMgr := &client.ClientManager{
		PaymentClient: mock,
	}
	return &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: clientMgr,
		},
	}
}

func stringPtr(s string) *string {
	return &s
}

func TestCancelPayment_Unauthenticated(t *testing.T) {
	mock := &mockPaymentClientForResolvers{}
	r := newTestPaymentResolver(mock)

	_, err := r.CancelPayment(context.Background(), stringPtr("pay_1"), nil, nil)
	if err == nil {
		t.Fatal("expected error for unauthenticated call, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED error, got %v", err)
	}
}

func TestCancelPayment_NilClient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	_, err := r.CancelPayment(ctx, stringPtr("pay_1"), nil, nil)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Errorf("expected INTERNAL_SERVER_ERROR error, got %v", err)
	}
}

func TestCancelPayment_MissingIDs(t *testing.T) {
	mock := &mockPaymentClientForResolvers{}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	_, err := r.CancelPayment(ctx, nil, nil, nil)
	if err == nil {
		t.Fatal("expected error when IDs are omitted, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST error, got %v", err)
	}
}

func TestCancelPayment_CustomerOwner_Success_ByPaymentID(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		getPaymentResp: &paymentpb.GetPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_1",
				OrderId: "ord_1",
				UserId:  "usr_alice",
				Status:  "PENDING",
				Amount:  50.0,
			},
		},
		cancelPaymentResp: &paymentpb.CancelPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_1",
				OrderId: "ord_1",
				UserId:  "usr_alice",
				Status:  "CANCELLED",
				Amount:  50.0,
			},
		},
	}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	res, err := r.CancelPayment(ctx, stringPtr("pay_1"), nil, stringPtr("Customer cancelled"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "pay_1" || res.Status != "CANCELLED" {
		t.Errorf("unexpected payment result: %+v", res)
	}
	if mock.lastGetReq == nil || mock.lastGetReq.Id != "pay_1" {
		t.Errorf("expected GetPayment with pay_1, got %+v", mock.lastGetReq)
	}
	if mock.lastCancelReq == nil || mock.lastCancelReq.PaymentId != "pay_1" || mock.lastCancelReq.Reason != "Customer cancelled" {
		t.Errorf("expected CancelPayment with pay_1, got %+v", mock.lastCancelReq)
	}
}

func TestCancelPayment_CustomerOwner_Success_ByOrderID(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		getByOrderIDResp: &paymentpb.GetPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_2",
				OrderId: "ord_2",
				UserId:  "usr_alice",
				Status:  "INITIATED",
				Amount:  75.0,
			},
		},
		cancelPaymentResp: &paymentpb.CancelPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_2",
				OrderId: "ord_2",
				UserId:  "usr_alice",
				Status:  "CANCELLED",
				Amount:  75.0,
			},
		},
	}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	res, err := r.CancelPayment(ctx, nil, stringPtr("ord_2"), stringPtr("Duplicate order"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "pay_2" || res.Status != "CANCELLED" {
		t.Errorf("unexpected payment result: %+v", res)
	}
	if mock.lastGetByOrderIDReq == nil || mock.lastGetByOrderIDReq.OrderId != "ord_2" {
		t.Errorf("expected GetPaymentByOrderID with ord_2, got %+v", mock.lastGetByOrderIDReq)
	}
	if mock.lastCancelReq == nil || mock.lastCancelReq.OrderId != "ord_2" {
		t.Errorf("expected CancelPayment with ord_2, got %+v", mock.lastCancelReq)
	}
}

func TestCancelPayment_CustomerNonOwner_Forbidden(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		getPaymentResp: &paymentpb.GetPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_1",
				OrderId: "ord_1",
				UserId:  "usr_alice", // owned by Alice
				Status:  "PENDING",
			},
		},
	}
	r := newTestPaymentResolver(mock)

	// Mallory tries to cancel Alice's payment
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_mallory",
		Role:   auth.RoleCustomer,
	})

	_, err := r.CancelPayment(ctx, stringPtr("pay_1"), nil, nil)
	if err == nil {
		t.Fatal("expected forbidden error for non-owner, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN error, got %v", err)
	}

	if mock.lastCancelReq != nil {
		t.Error("expected CancelPayment NOT to be called when authorization fails")
	}
}

func TestCancelPayment_Admin_Success_BypassesOwnershipCheck(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		cancelPaymentResp: &paymentpb.CancelPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_1",
				OrderId: "ord_1",
				UserId:  "usr_alice",
				Status:  "CANCELLED",
			},
		},
	}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin_user",
		Role:   auth.RoleAdmin,
	})

	res, err := r.CancelPayment(ctx, stringPtr("pay_1"), nil, stringPtr("Admin cancelled order"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "pay_1" || res.Status != "CANCELLED" {
		t.Errorf("unexpected payment result: %+v", res)
	}
	if mock.lastCancelReq == nil || mock.lastCancelReq.PaymentId != "pay_1" {
		t.Errorf("expected CancelPayment to be called with pay_1, got %+v", mock.lastCancelReq)
	}
	// Admin bypasses ownership check, so GetPayment is not called
	if mock.lastGetReq != nil {
		t.Errorf("expected GetPayment not to be called for ADMIN, got %+v", mock.lastGetReq)
	}
}

func TestCancelPayment_PaymentNotFound(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		getPaymentErr: status.Error(codes.NotFound, "payment not found"),
	}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	_, err := r.CancelPayment(ctx, stringPtr("pay_unknown"), nil, nil)
	if err == nil {
		t.Fatal("expected error for non-existent payment, got nil")
	}

	if mock.lastCancelReq != nil {
		t.Error("expected CancelPayment NOT to be called when payment is not found")
	}
}

func TestCancelPayment_BackendCancelFails(t *testing.T) {
	mock := &mockPaymentClientForResolvers{
		getPaymentResp: &paymentpb.GetPaymentResponse{
			Payment: &paymentpb.Payment{
				Id:      "pay_1",
				OrderId: "ord_1",
				UserId:  "usr_alice",
				Status:  "SUCCESS",
			},
		},
		cancelPaymentErr: status.Error(codes.FailedPrecondition, "cannot cancel a successful payment"),
	}
	r := newTestPaymentResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "usr_alice",
		Role:   auth.RoleCustomer,
	})

	_, err := r.CancelPayment(ctx, stringPtr("pay_1"), nil, nil)
	if err == nil {
		t.Fatal("expected error when backend cancellation fails, got nil")
	}
}
