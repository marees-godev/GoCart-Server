package resolvers

import (
	"context"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockUserClientForUserResolvers struct {
	userpb.UserServiceClient
	lastReactivateReq *userpb.ReactivateUserRequest
	reactivateErr     error
	updateErr         error
}

func (m *mockUserClientForUserResolvers) ReactivateUser(ctx context.Context, in *userpb.ReactivateUserRequest, opts ...grpcPkg.CallOption) (*userpb.ReactivateUserResponse, error) {
	m.lastReactivateReq = in
	if m.reactivateErr != nil {
		return nil, m.reactivateErr
	}
	return &userpb.ReactivateUserResponse{
		Success: true,
		Message: "account reactivated successfully",
		Status:  "active",
	}, nil
}

func (m *mockUserClientForUserResolvers) UpdateUser(ctx context.Context, in *userpb.UpdateUserRequest, opts ...grpcPkg.CallOption) (*userpb.UpdateUserResponse, error) {
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return &userpb.UpdateUserResponse{
		User: &userpb.User{
			Id: in.Id,
		},
	}, nil
}

func TestReactivateAccount_Success(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	res, err := r.ReactivateAccount(ctx)
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}

	if !res.Success || res.Status != "active" {
		t.Errorf("expected active status and success=true, got %+v", res)
	}

	if userMock.lastReactivateReq == nil || userMock.lastReactivateReq.Id != "user-123" {
		t.Errorf("expected reactivate request with id 'user-123', got %+v", userMock.lastReactivateReq)
	}
}

func TestReactivateAccount_Unauthenticated(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := context.Background()

	_, err := r.ReactivateAccount(ctx)
	if err == nil {
		t.Fatal("expected unauthorized error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got %s", appErr.Code)
	}
}

func TestReactivateAccount_PermissionDenied_MapsToForbidden(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		reactivateErr: status.Error(codes.PermissionDenied, "account deactivation period of 30 days has expired; account has been permanently deleted"),
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	_, err := r.ReactivateAccount(ctx)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 403 {
		t.Errorf("expected 403 HTTP status, got %d", appErr.HTTPStatus)
	}
}

func TestUpdateUser_InvalidArgument_MapsToBadRequest(t *testing.T) {
	userMock := &mockUserClientForUserResolvers{
		updateErr: status.Error(codes.InvalidArgument, "gender must be one of: male, female, others"),
	}
	clients := &grpc.Clients{
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   "CUSTOMER",
	})

	_, err := r.UpdateUser(ctx, "user-123", model.UpdateUserInput{})
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil {
		t.Fatalf("expected AppError, got %T: %v", err, err)
	}
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
	if appErr.HTTPStatus != 400 {
		t.Errorf("expected 400 HTTP status, got %d", appErr.HTTPStatus)
	}
}
