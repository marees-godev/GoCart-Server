package resolvers

import (
	"context"
	"testing"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
)

type mockUserClientForUserResolvers struct {
	userpb.UserServiceClient
	lastReactivateReq *userpb.ReactivateUserRequest
}

func (m *mockUserClientForUserResolvers) ReactivateUser(ctx context.Context, in *userpb.ReactivateUserRequest, opts ...grpcPkg.CallOption) (*userpb.ReactivateUserResponse, error) {
	m.lastReactivateReq = in
	return &userpb.ReactivateUserResponse{
		Success: true,
		Message: "account reactivated successfully",
		Status:  "active",
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

	// Unauthenticated context
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
