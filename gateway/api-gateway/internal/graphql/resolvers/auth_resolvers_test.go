package resolvers

import (
	"context"
	"testing"

	authpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	grpcPkg "google.golang.org/grpc"
)

type mockAuthClient struct {
	authpb.AuthServiceClient
	lastRegisterReq *authpb.RegisterRequest
}

func (m *mockAuthClient) Register(ctx context.Context, in *authpb.RegisterRequest, opts ...grpcPkg.CallOption) (*authpb.AuthResponse, error) {
	m.lastRegisterReq = in
	return &authpb.AuthResponse{
		AccessToken: "test-access-token",
		TokenType:   "Bearer",
		ExpiresIn:   900,
		UserId:      "user-uuid-123",
	}, nil
}

type mockUserClient struct {
	userpb.UserServiceClient
	lastGetUserReq *userpb.GetUserRequest
}

func (m *mockUserClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	m.lastGetUserReq = in
	return &userpb.GetUserResponse{
		User: &userpb.User{
			Id:        in.Id,
			Email:     "customer@example.com",
			FirstName: "John",
			LastName:  "Doe",
			CreatedAt: "2026-09-23T10:00:00Z",
		},
	}, nil
}

func TestRegisterResolver_Customer(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "John"
	ln := "Doe"
	isMerch := false
	input := model.RegisterInput{
		Email:      "customer@example.com",
		Password:   "Password123!",
		FirstName:  &fn,
		LastName:   &ln,
		IsMerchant: &isMerch,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful customer registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with IsMerchant == false
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.IsMerchant != false {
		t.Errorf("expected IsMerchant to be false, got true")
	}

	// Verify UserClient.GetUser was called
	if userMock.lastGetUserReq == nil || userMock.lastGetUserReq.Id != "user-uuid-123" {
		t.Errorf("expected UserClient.GetUser to be called with user-uuid-123")
	}

	// Verify payload.User is populated
	if payload.User == nil || payload.User.ID != "user-uuid-123" {
		t.Errorf("expected payload user with ID 'user-uuid-123', got %+v", payload.User)
	}
	if payload.User.CreatedAt == nil || *payload.User.CreatedAt != "2026-09-23T10:00:00Z" {
		t.Errorf("expected payload user with CreatedAt '2026-09-23T10:00:00Z', got %+v", payload.User)
	}
}

func TestRegisterResolver_Merchant(t *testing.T) {
	authMock := &mockAuthClient{}
	userMock := &mockUserClient{}

	clients := &grpc.Clients{
		AuthClient: authMock,
		UserClient: userMock,
	}

	r := &mutationResolver{
		Resolver: &Resolver{
			Clients: clients,
		},
	}

	fn := "Jane"
	ln := "Merchant"
	isMerch := true
	input := model.RegisterInput{
		Email:      "merchant@example.com",
		Password:   "Password123!",
		FirstName:  &fn,
		LastName:   &ln,
		IsMerchant: &isMerch,
	}

	payload, err := r.Register(context.Background(), input)
	if err != nil {
		t.Fatalf("expected successful merchant registration, got err: %v", err)
	}

	if payload.Token != "test-access-token" {
		t.Errorf("expected token 'test-access-token', got '%s'", payload.Token)
	}

	// Verify AuthClient called with IsMerchant == true
	if authMock.lastRegisterReq == nil {
		t.Fatal("expected AuthClient.Register to be called")
	}
	if authMock.lastRegisterReq.IsMerchant != true {
		t.Errorf("expected IsMerchant to be true, got false")
	}

	// Verify UserClient.GetUser was NOT called for merchant
	if userMock.lastGetUserReq != nil {
		t.Errorf("expected UserClient.GetUser not to be called for merchant")
	}

	// Verify payload.User is populated and not null
	if payload.User == nil {
		t.Fatal("expected non-null user in AuthPayload")
	}
	if payload.User.ID != "user-uuid-123" {
		t.Errorf("expected user ID 'user-uuid-123', got '%s'", payload.User.ID)
	}
	if payload.User.Email != "merchant@example.com" {
		t.Errorf("expected email 'merchant@example.com', got '%s'", payload.User.Email)
	}
	if payload.User.FirstName == nil || *payload.User.FirstName != "Jane" || payload.User.LastName == nil || *payload.User.LastName != "Merchant" {
		t.Errorf("expected name Jane Merchant, got %v %v", payload.User.FirstName, payload.User.LastName)
	}
}
