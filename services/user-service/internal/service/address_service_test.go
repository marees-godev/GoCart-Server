package service_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type mockAddressRepository struct {
	addresses map[string]*model.Address
	idCounter int
}

func newMockAddressRepo() *mockAddressRepository {
	return &mockAddressRepository{
		addresses: make(map[string]*model.Address),
	}
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

func setupAddressTestService() (service.AddressService, *mockUserRepository, *mockAddressRepository) {
	userRepo := newMockRepo()
	addressRepo := newMockAddressRepo()

	user := &model.User{
		ID:        "user-1",
		Email:     "user1@example.com",
		FirstName: "Alex",
		LastName:  "Morgan",
		Status:    "active",
	}
	user2 := &model.User{
		ID:        "user-2",
		Email:     "user2@example.com",
		FirstName: "Jane",
		LastName:  "Smith",
		Status:    "active",
	}
	userRepo.users[user.ID] = user
	userRepo.users[user2.ID] = user2

	svc := service.NewAddressService(addressRepo, userRepo)
	return svc, userRepo, addressRepo
}

func TestCreateAddress_SuccessAndMultipleAddresses(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	labelHome := "Home"
	emailAddress := "alex.morgan@example.com"
	req1 := dto.CreateAddressRequest{
		Label:        &labelHome,
		FullName:     "Alex Morgan",
		PhoneNumber:  "+1 (555) 234-5678",
		EmailAddress: &emailAddress,
		AddressLine:  "742 Evergreen Terrace",
		City:         "Springfield",
		State:        "OR",
		PostalCode:   "97477",
		Country:      "United States",
	}

	res1, err := svc.CreateAddress(context.Background(), "user-1", req1)
	if err != nil {
		t.Fatalf("expected no error creating address 1, got %v", err)
	}
	if !res1.IsDefault {
		t.Errorf("expected first address to be default, got false")
	}
	if res1.FullName == nil || *res1.FullName != "Alex Morgan" {
		t.Errorf("expected FullName Alex Morgan, got %v", res1.FullName)
	}
	if res1.PhoneNumber == nil || *res1.PhoneNumber != "+1 (555) 234-5678" {
		t.Errorf("expected PhoneNumber +1 (555) 234-5678, got %v", res1.PhoneNumber)
	}
	if res1.EmailAddress == nil || *res1.EmailAddress != "alex.morgan@example.com" {
		t.Errorf("expected EmailAddress alex.morgan@example.com, got %v", res1.EmailAddress)
	}

	labelWork := "Work"
	req2 := dto.CreateAddressRequest{
		Label:       &labelWork,
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 876-5432",
		AddressLine: "100 Silicon Ave, Suite 400",
		City:        "San Francisco",
		State:       "CA",
		PostalCode:  "94107",
		Country:     "United States",
	}

	res2, err := svc.CreateAddress(context.Background(), "user-1", req2)
	if err != nil {
		t.Fatalf("expected no error creating address 2, got %v", err)
	}
	if res2.IsDefault {
		t.Errorf("expected second address not to be default by default, got true")
	}
}

func TestCreateAddress_AddressTypesNormalization(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	tests := []struct {
		name          string
		inputLabel    *string
		expectedLabel string
	}{
		{
			name:          "Default to Home when nil",
			inputLabel:    nil,
			expectedLabel: "Home",
		},
		{
			name:          "Normalize lowercase home to Home",
			inputLabel:    func() *string { s := "home"; return &s }(),
			expectedLabel: "Home",
		},
		{
			name:          "Normalize uppercase WORK to Work",
			inputLabel:    func() *string { s := "WORK"; return &s }(),
			expectedLabel: "Work",
		},
		{
			name:          "Normalize mixed-case oThEr to Other",
			inputLabel:    func() *string { s := "oThEr"; return &s }(),
			expectedLabel: "Other",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := dto.CreateAddressRequest{
				Label:       tt.inputLabel,
				FullName:    "Test User",
				PhoneNumber: "+1 (555) 000-0000",
				AddressLine: "123 Main St",
				City:        "Springfield",
				State:       "OR",
				PostalCode:  "97477",
				Country:     "United States",
			}
			res, err := svc.CreateAddress(context.Background(), "user-1", req)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Label == nil || *res.Label != tt.expectedLabel {
				t.Errorf("expected label %q, got %v", tt.expectedLabel, res.Label)
			}
		})
	}
}

func TestSetDefaultAddress_SuccessAndOwnership(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 234-5678",
		AddressLine: "742 Evergreen Terrace",
		City:        "Springfield",
		State:       "OR",
		PostalCode:  "97477",
		Country:     "United States",
	})

	addr2, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "Alex Morgan",
		PhoneNumber: "+1 (555) 234-5678",
		AddressLine: "100 Silicon Ave",
		City:        "San Francisco",
		State:       "CA",
		PostalCode:  "94107",
		Country:     "United States",
	})

	if !addr1.IsDefault {
		t.Errorf("expected address 1 to initially be default")
	}

	_, err := svc.SetDefaultAddress(context.Background(), "user-2", addr2.ID)
	if err == nil {
		t.Fatal("expected error when user-2 sets user-1's address as default")
	}

	updated2, err := svc.SetDefaultAddress(context.Background(), "user-1", addr2.ID)
	if err != nil {
		t.Fatalf("expected success setting default address, got %v", err)
	}
	if !updated2.IsDefault {
		t.Errorf("expected address 2 to be default now")
	}

	fetch1, _ := svc.GetAddress(context.Background(), "user-1", addr1.ID)
	if fetch1.IsDefault {
		t.Errorf("expected address 1 to no longer be default")
	}
}

func TestListAddresses_Success(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "100 Elm St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "200 Oak St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})

	addresses, err := svc.ListAddresses(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("expected no error listing addresses, got %v", err)
	}
	if len(addresses) != 2 {
		t.Errorf("expected 2 addresses for user-1, got %d", len(addresses))
	}
}

func TestGetAddress_OwnershipValidation(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	created, err := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "100 Elm St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	_, err = svc.GetAddress(context.Background(), "user-2", created.ID)
	if err == nil {
		t.Fatal("expected error when user-2 accesses user-1's address, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestUpdateAddress_SuccessAndUnauthorized(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	created, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "100 Original St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	newLine := "100 Updated St"
	newCity := "Round Rock"
	updateReq := dto.UpdateAddressRequest{
		AddressLine: &newLine,
		City:        &newCity,
	}

	_, err := svc.UpdateAddress(context.Background(), "user-2", created.ID, updateReq)
	if err == nil {
		t.Fatal("expected error when user-2 updates user-1's address, got nil")
	}

	updated, err := svc.UpdateAddress(context.Background(), "user-1", created.ID, updateReq)
	if err != nil {
		t.Fatalf("expected update success, got %v", err)
	}
	if updated.AddressLine != "100 Updated St" || updated.City != "Round Rock" {
		t.Errorf("address fields not updated correctly: %+v", updated)
	}
}

func TestDeleteAddress_SuccessAndIsolation(t *testing.T) {
	svc, _, addressRepo := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "User 1 Address",
		City:        "City A",
		State:       "State A",
		PostalCode:  "12345",
		Country:     "United States",
	})

	addr2, _ := svc.CreateAddress(context.Background(), "user-2", dto.CreateAddressRequest{
		FullName:    "Jane Smith",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "User 2 Address",
		City:        "City B",
		State:       "State B",
		PostalCode:  "54321",
		Country:     "United States",
	})

	err := svc.DeleteAddress(context.Background(), "user-1", addr2.ID)
	if err == nil {
		t.Fatal("expected error when user-1 attempts to delete user-2's address")
	}

	_, err = addressRepo.GetByID(context.Background(), addr2.ID)
	if err != nil {
		t.Errorf("user-2's address was affected: %v", err)
	}

	err = svc.DeleteAddress(context.Background(), "user-1", addr1.ID)
	if err != nil {
		t.Fatalf("expected successful deletion of own address, got %v", err)
	}

	_, err = svc.GetAddress(context.Background(), "user-1", addr1.ID)
	if err == nil {
		t.Fatal("expected address 1 to be deleted")
	}
}

func TestAddress_ValidationAndNonExistent(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	invalidReq := dto.CreateAddressRequest{
		FullName:    "John Doe",
		PhoneNumber: "+1 (555) 000-0000",
		AddressLine: "123 Main St",
		City:        "City",
		State:       "State",
		PostalCode:  "!@#$%^",
		Country:     "Country",
	}
	_, err := svc.CreateAddress(context.Background(), "user-1", invalidReq)
	if err == nil {
		t.Fatal("expected validation error for invalid postal code")
	}

	_, err = svc.GetAddress(context.Background(), "user-1", "non-existent-id")
	if err == nil {
		t.Fatal("expected error for non-existent address")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND error, got %s", appErr.Code)
	}
}
