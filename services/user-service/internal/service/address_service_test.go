package service_test

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type mockAddressRepository struct {
	mu        sync.Mutex
	addresses map[string]*model.Address
	idCounter int
}

func newMockAddressRepo() *mockAddressRepository {
	return &mockAddressRepository{
		addresses: make(map[string]*model.Address),
	}
}

func (m *mockAddressRepository) Create(ctx context.Context, address *model.Address) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.idCounter++
	address.ID = fmt.Sprintf("addr-%d", m.idCounter)
	address.CreatedAt = time.Now()
	address.UpdatedAt = time.Now()

	count := 0
	for _, a := range m.addresses {
		if a.UserID == address.UserID {
			count++
		}
	}
	if count == 0 {
		address.IsDefault = true
	}
	if address.IsDefault {
		for _, a := range m.addresses {
			if a.UserID == address.UserID {
				a.IsDefault = false
			}
		}
	}

	copy := *address
	m.addresses[address.ID] = &copy
	return nil
}

func (m *mockAddressRepository) GetByID(ctx context.Context, id string) (*model.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	a, exists := m.addresses[id]
	if !exists {
		return nil, appErrors.NotFound("address not found")
	}
	copy := *a
	return &copy, nil
}

func (m *mockAddressRepository) ListByUserID(ctx context.Context, userID string) ([]*model.Address, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.addresses[address.ID]
	if !exists || existing.UserID != address.UserID {
		return appErrors.NotFound("address not found")
	}
	if address.IsDefault {
		for _, a := range m.addresses {
			if a.UserID == address.UserID && a.ID != address.ID {
				a.IsDefault = false
			}
		}
	}
	address.UpdatedAt = time.Now()
	copy := *address
	m.addresses[address.ID] = &copy
	return nil
}

func (m *mockAddressRepository) Delete(ctx context.Context, id string, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	existing, exists := m.addresses[id]
	if !exists || existing.UserID != userID {
		return appErrors.NotFound("address not found")
	}
	wasDefault := existing.IsDefault
	delete(m.addresses, id)

	if wasDefault {
		var candidate *model.Address
		for _, a := range m.addresses {
			if a.UserID == userID {
				if candidate == nil || a.UpdatedAt.After(candidate.UpdatedAt) {
					candidate = a
				}
			}
		}
		if candidate != nil {
			candidate.IsDefault = true
			candidate.UpdatedAt = time.Now()
		}
	}
	return nil
}

func (m *mockAddressRepository) ClearDefaultAddresses(ctx context.Context, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, a := range m.addresses {
		if a.UserID == userID {
			a.IsDefault = false
		}
	}
	return nil
}

func (m *mockAddressRepository) SetDefaultAddress(ctx context.Context, id string, userID string) error {
	m.mu.Lock()
	defer m.mu.Unlock()

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
	m.mu.Lock()
	defer m.mu.Unlock()

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

func TestCreateAddress_FirstAddressAlwaysDefaultEvenIfFalseRequested(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	falseVal := false
	req := dto.CreateAddressRequest{
		FullName:    "First User",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "123 First St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
		IsDefault:   &falseVal,
	}

	addr, err := svc.CreateAddress(context.Background(), "user-1", req)
	if err != nil {
		t.Fatalf("unexpected error creating first address: %v", err)
	}

	if !addr.IsDefault {
		t.Fatalf("expected first address to be forced to default, got false")
	}
}

func TestCreateAddress_SubsequentAddressWithDefaultClearsPrevious(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, err := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "First User",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "123 First St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	if err != nil {
		t.Fatalf("unexpected error creating addr1: %v", err)
	}
	if !addr1.IsDefault {
		t.Fatalf("expected addr1 to be default")
	}

	trueVal := true
	addr2, err := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "First User",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "456 Second St",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
		IsDefault:   &trueVal,
	})
	if err != nil {
		t.Fatalf("unexpected error creating addr2: %v", err)
	}
	if !addr2.IsDefault {
		t.Fatalf("expected addr2 to be default")
	}

	refreshed1, err := svc.GetAddress(context.Background(), "user-1", addr1.ID)
	if err != nil {
		t.Fatalf("unexpected error fetching addr1: %v", err)
	}
	if refreshed1.IsDefault {
		t.Fatalf("expected addr1 default status to be removed, got true")
	}
}

func TestDeleteAddress_DefaultAddressPromotesRemainingAddress(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 1",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	addr2, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 2",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})

	if !addr1.IsDefault || addr2.IsDefault {
		t.Fatalf("expected addr1 to be default and addr2 non-default")
	}

	if err := svc.DeleteAddress(context.Background(), "user-1", addr1.ID); err != nil {
		t.Fatalf("failed to delete default address: %v", err)
	}

	refreshed2, err := svc.GetAddress(context.Background(), "user-1", addr2.ID)
	if err != nil {
		t.Fatalf("failed to get remaining address: %v", err)
	}
	if !refreshed2.IsDefault {
		t.Fatalf("expected remaining address to be promoted to default, got false")
	}
}

func TestDeleteAddress_OnlyAddressDeletesCleanly(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Single Address",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	if err := svc.DeleteAddress(context.Background(), "user-1", addr1.ID); err != nil {
		t.Fatalf("failed to delete address: %v", err)
	}

	list, err := svc.ListAddresses(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("failed to list addresses: %v", err)
	}
	if len(list) != 0 {
		t.Fatalf("expected 0 addresses, got %d", len(list))
	}
}

func TestDeleteAddress_NonDefaultLeavesDefaultIntact(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 1",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	addr2, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 2",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})

	if err := svc.DeleteAddress(context.Background(), "user-1", addr2.ID); err != nil {
		t.Fatalf("failed to delete non-default address: %v", err)
	}

	refreshed1, err := svc.GetAddress(context.Background(), "user-1", addr1.ID)
	if err != nil {
		t.Fatalf("failed to get default address: %v", err)
	}
	if !refreshed1.IsDefault {
		t.Fatalf("expected addr1 to remain default")
	}
}

func TestUpdateAddress_CannotUnsetDefaultDirectly(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 1",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})

	falseVal := false
	_, err := svc.UpdateAddress(context.Background(), "user-1", addr1.ID, dto.UpdateAddressRequest{
		IsDefault: &falseVal,
	})
	if err == nil {
		t.Fatalf("expected error when attempting to unset default address directly, got nil")
	}

	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Fatalf("expected BAD_REQUEST error code, got %s", appErr.Code)
	}
}

func TestUpdateAddress_SettingDefaultClearsPreviousDefault(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 1",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	addr2, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 2",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})

	trueVal := true
	updated2, err := svc.UpdateAddress(context.Background(), "user-1", addr2.ID, dto.UpdateAddressRequest{
		IsDefault: &trueVal,
	})
	if err != nil {
		t.Fatalf("failed to update address 2 to default: %v", err)
	}
	if !updated2.IsDefault {
		t.Fatalf("expected address 2 to be default")
	}

	refreshed1, err := svc.GetAddress(context.Background(), "user-1", addr1.ID)
	if err != nil {
		t.Fatalf("failed to get address 1: %v", err)
	}
	if refreshed1.IsDefault {
		t.Fatalf("expected address 1 to no longer be default")
	}
}

func TestSetDefaultAddress_ConcurrentUpdates(t *testing.T) {
	svc, _, _ := setupAddressTestService()

	addr1, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 1",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78701",
		Country:     "United States",
	})
	addr2, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 2",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78702",
		Country:     "United States",
	})
	addr3, _ := svc.CreateAddress(context.Background(), "user-1", dto.CreateAddressRequest{
		FullName:    "User 1",
		PhoneNumber: "+1 (555) 111-2222",
		AddressLine: "Address 3",
		City:        "Austin",
		State:       "TX",
		PostalCode:  "78703",
		Country:     "United States",
	})

	targets := []string{addr1.ID, addr2.ID, addr3.ID}

	var wg sync.WaitGroup
	for i := 0; i < 20; i++ {
		targetID := targets[i%len(targets)]
		wg.Add(1)
		go func(id string) {
			defer wg.Done()
			_, _ = svc.SetDefaultAddress(context.Background(), "user-1", id)
		}(targetID)
	}
	wg.Wait()

	addresses, err := svc.ListAddresses(context.Background(), "user-1")
	if err != nil {
		t.Fatalf("failed to list addresses: %v", err)
	}

	defaultCount := 0
	for _, a := range addresses {
		if a.IsDefault {
			defaultCount++
		}
	}

	if defaultCount != 1 {
		t.Fatalf("expected exactly 1 default address, found %d", defaultCount)
	}
}

