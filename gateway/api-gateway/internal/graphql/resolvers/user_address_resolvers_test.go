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

type mockUserAddressClient struct {
	userpb.UserServiceClient
	users             map[string]*userpb.User
	createAddressHook func(ctx context.Context, in *userpb.CreateUserAddressRequest) (*userpb.CreateUserAddressResponse, error)
}

func newMockUserAddressClient() *mockUserAddressClient {
	return &mockUserAddressClient{
		users: make(map[string]*userpb.User),
	}
}

func (m *mockUserAddressClient) GetUser(ctx context.Context, in *userpb.GetUserRequest, opts ...grpcPkg.CallOption) (*userpb.GetUserResponse, error) {
	u, ok := m.users[in.Id]
	if !ok {
		return nil, status.Error(codes.NotFound, "user not found")
	}
	return &userpb.GetUserResponse{User: u}, nil
}

func (m *mockUserAddressClient) CreateUser(ctx context.Context, in *userpb.CreateUserRequest, opts ...grpcPkg.CallOption) (*userpb.CreateUserResponse, error) {
	u := &userpb.User{
		Id:        in.Id,
		Email:     in.Email,
		FirstName: in.FirstName,
		LastName:  in.LastName,
		Status:    "ACTIVE",
	}
	m.users[in.Id] = u
	return &userpb.CreateUserResponse{User: u}, nil
}

func (m *mockUserAddressClient) CreateUserAddress(ctx context.Context, in *userpb.CreateUserAddressRequest, opts ...grpcPkg.CallOption) (*userpb.CreateUserAddressResponse, error) {
	if m.createAddressHook != nil {
		return m.createAddressHook(ctx, in)
	}
	addr := &userpb.Address{
		Id:           "addr-test-1",
		UserId:       in.UserId,
		Label:        in.Label,
		FullName:     in.FullName,
		PhoneNumber:  in.PhoneNumber,
		EmailAddress: in.EmailAddress,
		AddressLine:  in.AddressLine,
		City:         in.City,
		State:        in.State,
		PostalCode:   in.PostalCode,
		Country:      in.Country,
		IsDefault:    in.IsDefault,
		CreatedAt:    "2026-11-19T10:00:00Z",
		UpdatedAt:    "2026-11-19T10:00:00Z",
	}
	return &userpb.CreateUserAddressResponse{Address: addr}, nil
}

func TestCreateUserAddress_Success(t *testing.T) {
	client := newMockUserAddressClient()
	userID := "01a1ed98-f4f0-754e-b19c-e2ed887d2cbb"
	client.users[userID] = &userpb.User{
		Id:     userID,
		Email:  "nikotest122@gmail.com",
		Status: "ACTIVE",
	}

	clients := grpc.NewClientsWithServices(client)
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   "CUSTOMER",
		Email:  "nikotest122@gmail.com",
	})

	label := "Home"
	email := "nikotest122@gmail.com"
	isDefault := false

	input := model.CreateAddressInput{
		Label:        &label,
		FullName:     "test mail",
		PhoneNumber:  "9514044440",
		EmailAddress: &email,
		AddressLine:  "laksmipuram",
		City:         "ramnad",
		State:        "Tamil Nadu",
		PostalCode:   "623517",
		Country:      "United States",
		IsDefault:    &isDefault,
	}

	addr, err := resolver.Mutation().CreateUserAddress(ctx, input)
	if err != nil {
		t.Fatalf("expected successful address creation without userId input, got: %v", err)
	}

	if addr == nil {
		t.Fatal("expected non-nil address returned")
	}
	if addr.ID != "addr-test-1" {
		t.Errorf("expected ID addr-test-1, got %s", addr.ID)
	}
	if addr.UserID != userID {
		t.Errorf("expected UserID %s from JWT auth context, got %s", userID, addr.UserID)
	}
	if addr.Label == nil || *addr.Label != "Home" {
		t.Errorf("expected Label Home, got %v", addr.Label)
	}
	if addr.FullName == nil || *addr.FullName != "test mail" {
		t.Errorf("expected FullName 'test mail', got %v", addr.FullName)
	}
	if addr.PhoneNumber == nil || *addr.PhoneNumber != "9514044440" {
		t.Errorf("expected PhoneNumber 9514044440, got %v", addr.PhoneNumber)
	}
	if addr.EmailAddress == nil || *addr.EmailAddress != "nikotest122@gmail.com" {
		t.Errorf("expected EmailAddress 'nikotest122@gmail.com', got %v", addr.EmailAddress)
	}
	if addr.AddressLine != "laksmipuram" {
		t.Errorf("expected AddressLine 'laksmipuram', got %s", addr.AddressLine)
	}
	if addr.City != "ramnad" {
		t.Errorf("expected City 'ramnad', got %s", addr.City)
	}
	if addr.State != "Tamil Nadu" {
		t.Errorf("expected State 'Tamil Nadu', got %s", addr.State)
	}
	if addr.PostalCode != "623517" {
		t.Errorf("expected PostalCode '623517', got %s", addr.PostalCode)
	}
	if addr.Country != "United States" {
		t.Errorf("expected Country 'United States', got %s", addr.Country)
	}
	if addr.IsDefault != false {
		t.Errorf("expected IsDefault false, got %v", addr.IsDefault)
	}
	if addr.CreatedAt == nil || *addr.CreatedAt != "2026-11-19T10:00:00Z" {
		t.Errorf("expected CreatedAt timestamp, got %v", addr.CreatedAt)
	}
	if addr.UpdatedAt == nil || *addr.UpdatedAt != "2026-11-19T10:00:00Z" {
		t.Errorf("expected UpdatedAt timestamp, got %v", addr.UpdatedAt)
	}
}

func TestCreateUserAddress_Unauthenticated(t *testing.T) {
	client := newMockUserAddressClient()
	clients := grpc.NewClientsWithServices(client)
	resolver := NewResolver(clients)

	input := model.CreateAddressInput{
		FullName:    "Test User",
		PhoneNumber: "1234567890",
		AddressLine: "123 Street",
		City:        "City",
		State:       "State",
		PostalCode:  "12345",
		Country:     "United States",
	}

	_, err := resolver.Mutation().CreateUserAddress(context.Background(), input)
	if err == nil {
		t.Fatal("expected error when unauthenticated")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeUnauthorized {
		t.Errorf("expected UNAUTHORIZED code, got: %v", err)
	}
}

func TestCreateUserAddress_DownstreamValidationFailurePropagates(t *testing.T) {
	client := newMockUserAddressClient()
	client.createAddressHook = func(ctx context.Context, in *userpb.CreateUserAddressRequest) (*userpb.CreateUserAddressResponse, error) {
		return nil, status.Error(codes.InvalidArgument, "invalid phone_number format")
	}

	userID := "user-valid"
	clients := grpc.NewClientsWithServices(client)
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: userID,
		Role:   "CUSTOMER",
	})

	input := model.CreateAddressInput{
		FullName:    "Test User",
		PhoneNumber: "invalid-phone",
		AddressLine: "123 Street",
		City:        "City",
		State:       "State",
		PostalCode:  "12345",
		Country:     "United States",
	}

	_, err := resolver.Mutation().CreateUserAddress(ctx, input)
	if err == nil {
		t.Fatal("expected error when downstream validation fails")
	}

	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got: %v", err)
	}
	if appErr.Message != "invalid phone_number format" {
		t.Errorf("expected 'invalid phone_number format', got: %s", appErr.Message)
	}
}
