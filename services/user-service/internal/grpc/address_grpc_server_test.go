package grpc_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	userGRPC "github.com/marees-godev/GoCart-Server/services/user-service/internal/grpc"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockUserRepository struct {
	users map[string]*model.User
}

func newMockUserRepo() *mockUserRepository {
	return &mockUserRepository{users: make(map[string]*model.User)}
}

func (m *mockUserRepository) Create(ctx context.Context, u *model.User) error {
	m.users[u.ID] = u
	return nil
}

func (m *mockUserRepository) GetByID(ctx context.Context, id string) (*model.User, error) {
	u, exists := m.users[id]
	if !exists {
		return nil, appErrors.NotFound("user not found")
	}
	copy := *u
	return &copy, nil
}

func (m *mockUserRepository) GetByEmail(ctx context.Context, email string) (*model.User, error) {
	for _, u := range m.users {
		if u.Email == email {
			copy := *u
			return &copy, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (m *mockUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	for _, u := range m.users {
		if u.Username != nil && *u.Username == username {
			copy := *u
			return &copy, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (m *mockUserRepository) UpdateUser(ctx context.Context, u *model.User) error {
	m.users[u.ID] = u
	return nil
}


type mockAddressRepository struct {
	addresses map[string]*model.Address
	idCounter int
}

func newMockAddressRepo() *mockAddressRepository {
	return &mockAddressRepository{addresses: make(map[string]*model.Address)}
}

func (m *mockAddressRepository) Create(ctx context.Context, address *model.Address) error {
	m.idCounter++
	address.ID = fmt.Sprintf("addr-%d", m.idCounter)
	address.CreatedAt = time.Now()
	address.UpdatedAt = time.Now()
	copy := *address
	m.addresses[address.ID] = &copy
	return nil
}

func (m *mockAddressRepository) GetByID(ctx context.Context, id string) (*model.Address, error) {
	a, exists := m.addresses[id]
	if !exists {
		return nil, appErrors.NotFound("address not found")
	}
	copy := *a
	return &copy, nil
}

func (m *mockAddressRepository) ListByUserID(ctx context.Context, userID string) ([]*model.Address, error) {
	result := make([]*model.Address, 0)
	for _, a := range m.addresses {
		if a.UserID == userID {
			copy := *a
			result = append(result, &copy)
		}
	}
	return result, nil
}

func (m *mockAddressRepository) Update(ctx context.Context, address *model.Address) error {
	existing, exists := m.addresses[address.ID]
	if !exists || existing.UserID != address.UserID {
		return appErrors.NotFound("address not found")
	}
	address.UpdatedAt = time.Now()
	copy := *address
	m.addresses[address.ID] = &copy
	return nil
}

func (m *mockAddressRepository) Delete(ctx context.Context, id string, userID string) error {
	existing, exists := m.addresses[id]
	if !exists || existing.UserID != userID {
		return appErrors.NotFound("address not found")
	}
	delete(m.addresses, id)
	return nil
}

func (m *mockAddressRepository) ClearDefaultAddresses(ctx context.Context, userID string) error {
	for _, a := range m.addresses {
		if a.UserID == userID {
			a.IsDefault = false
		}
	}
	return nil
}

func (m *mockAddressRepository) SetDefaultAddress(ctx context.Context, id string, userID string) error {
	target, exists := m.addresses[id]
	if !exists || target.UserID != userID {
		return appErrors.NotFound("address not found")
	}
	for _, a := range m.addresses {
		if a.UserID == userID {
			a.IsDefault = false
		}
	}
	target.IsDefault = true
	target.UpdatedAt = time.Now()
	return nil
}

func (m *mockAddressRepository) CountByUserID(ctx context.Context, userID string) (int, error) {
	count := 0
	for _, a := range m.addresses {
		if a.UserID == userID {
			count++
		}
	}
	return count, nil
}

func setupGRPCTestServer() (*userGRPC.UserGRPCServer, *mockUserRepository, *mockAddressRepository) {
	userRepo := newMockUserRepo()
	addressRepo := newMockAddressRepo()

	userRepo.users["user-1"] = &model.User{
		ID:        "user-1",
		Email:     "user1@example.com",
		FirstName: "Alex",
		LastName:  "Morgan",
		Status:    "active",
	}
	userRepo.users["user-2"] = &model.User{
		ID:        "user-2",
		Email:     "user2@example.com",
		FirstName: "Jane",
		LastName:  "Smith",
		Status:    "active",
	}

	addressSvc := service.NewAddressService(addressRepo, userRepo)
	userSvc := service.NewUserService(userRepo)
	grpcServer := userGRPC.NewUserGRPCServer(userSvc, addressSvc)

	return grpcServer, userRepo, addressRepo
}

func TestGRPC_CreateUserAddress_SuccessAndValidation(t *testing.T) {
	server, _, _ := setupGRPCTestServer()
	ctx := context.Background()

	req := &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		Label:       "Home",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "742 Evergreen Terrace",
		City:        "Springfield",
		State:       "OR",
		PostalCode:  "97477",
		Country:     "United States",
	}

	res, err := server.CreateUserAddress(ctx, req)
	if err != nil {
		t.Fatalf("expected success, got error: %v", err)
	}
	if res.Address == nil || res.Address.Id == "" {
		t.Fatal("expected returned address to have ID")
	}
	if !res.Address.IsDefault {
		t.Errorf("expected first created address to be set as default")
	}

	invalidReq := &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "invalid-phone",
		AddressLine: "123 Main St",
		City:        "Springfield",
		State:       "OR",
		PostalCode:  "97477",
		Country:     "United States",
	}
	_, err = server.CreateUserAddress(ctx, invalidReq)
	if err == nil {
		t.Fatal("expected validation error for invalid phone number")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument code, got %v", st.Code())
	}
}

func TestGRPC_ListUserAddresses(t *testing.T) {
	server, _, _ := setupGRPCTestServer()
	ctx := context.Background()

	server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "100 Main St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "200 Second St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})

	res, err := server.ListUserAddresses(ctx, &userpb.ListUserAddressesRequest{UserId: "user-1"})
	if err != nil {
		t.Fatalf("unexpected error listing addresses: %v", err)
	}
	if len(res.Addresses) != 2 {
		t.Errorf("expected 2 addresses, got %d", len(res.Addresses))
	}
}

func TestGRPC_GetUserAddress_OwnershipAndNotFound(t *testing.T) {
	server, _, _ := setupGRPCTestServer()
	ctx := context.Background()

	created, _ := server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "100 Main St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	// Get own address
	got, err := server.GetUserAddress(ctx, &userpb.GetUserAddressRequest{
		UserId:    "user-1",
		AddressId: created.Address.Id,
	})
	if err != nil {
		t.Fatalf("unexpected error getting own address: %v", err)
	}
	if got.Address.AddressLine != "100 Main St" {
		t.Errorf("unexpected address line: %s", got.Address.AddressLine)
	}

	// Access another user's address -> PermissionDenied
	_, err = server.GetUserAddress(ctx, &userpb.GetUserAddressRequest{
		UserId:    "user-2",
		AddressId: created.Address.Id,
	})
	if err == nil {
		t.Fatal("expected error accessing another user's address")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}

	// Access non-existent address -> NotFound
	_, err = server.GetUserAddress(ctx, &userpb.GetUserAddressRequest{
		UserId:    "user-1",
		AddressId: "non-existent-id",
	})
	if err == nil {
		t.Fatal("expected error for non-existent address")
	}
	st, _ = status.FromError(err)
	if st.Code() != codes.NotFound {
		t.Errorf("expected NotFound, got %v", st.Code())
	}
}

func TestGRPC_UpdateUserAddress_SuccessAndIsolation(t *testing.T) {
	server, _, _ := setupGRPCTestServer()
	ctx := context.Background()

	created, _ := server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "100 Main St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	newCity := "Round Rock"
	updateReq := &userpb.UpdateUserAddressRequest{
		UserId:    "user-1",
		AddressId: created.Address.Id,
		City:      &newCity,
	}

	updated, err := server.UpdateUserAddress(ctx, updateReq)
	if err != nil {
		t.Fatalf("unexpected error updating address: %v", err)
	}
	if updated.Address.City != "Round Rock" {
		t.Errorf("expected city Round Rock, got %s", updated.Address.City)
	}

	// Attempt update by user-2 -> PermissionDenied
	_, err = server.UpdateUserAddress(ctx, &userpb.UpdateUserAddressRequest{
		UserId:    "user-2",
		AddressId: created.Address.Id,
		City:      &newCity,
	})
	if err == nil {
		t.Fatal("expected PermissionDenied updating another user's address")
	}
	st, _ := status.FromError(err)
	if st.Code() != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied, got %v", st.Code())
	}
}

func TestGRPC_DeleteUserAddress_SuccessAndIsolation(t *testing.T) {
	server, _, addressRepo := setupGRPCTestServer()
	ctx := context.Background()

	addr1, _ := server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-1",
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 123-4567",
		AddressLine: "User 1 Address",
		City:        "City A",
		State:       "State A",
		PostalCode:  "12345",
		Country:     "United States",
	})

	addr2, _ := server.CreateUserAddress(ctx, &userpb.CreateUserAddressRequest{
		UserId:      "user-2",
		FullName:    "Jane Smith",
		PhoneNumber: "+1 (555) 987-6543",
		AddressLine: "User 2 Address",
		City:        "City B",
		State:       "State B",
		PostalCode:  "54321",
		Country:     "United States",
	})

	// User-1 tries to delete User-2's address -> PermissionDenied
	_, err := server.DeleteUserAddress(ctx, &userpb.DeleteUserAddressRequest{
		UserId:    "user-1",
		AddressId: addr2.Address.Id,
	})
	if err == nil {
		t.Fatal("expected error deleting another user's address")
	}

	// Verify User-2's address still exists
	_, err = addressRepo.GetByID(ctx, addr2.Address.Id)
	if err != nil {
		t.Fatalf("user-2's address was incorrectly modified/deleted: %v", err)
	}

	// User-1 deletes own address -> Success
	delRes, err := server.DeleteUserAddress(ctx, &userpb.DeleteUserAddressRequest{
		UserId:    "user-1",
		AddressId: addr1.Address.Id,
	})
	if err != nil {
		t.Fatalf("unexpected error deleting own address: %v", err)
	}
	if !delRes.Success {
		t.Errorf("expected success true")
	}
}
