package service_test

import (
	"context"
	"strings"
	"testing"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/service"
)

type mockUserRepository struct {
	users map[string]*model.User
}

func newMockRepo() *mockUserRepository {
	return &mockUserRepository{
		users: make(map[string]*model.User),
	}
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
		if strings.EqualFold(u.Email, email) {
			copy := *u
			return &copy, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (m *mockUserRepository) GetByUsername(ctx context.Context, username string) (*model.User, error) {
	for _, u := range m.users {
		if u.Username != nil && strings.EqualFold(*u.Username, username) {
			copy := *u
			return &copy, nil
		}
	}
	return nil, appErrors.NotFound("user not found")
}

func (m *mockUserRepository) UpdateUser(ctx context.Context, user *model.User) error {
	existing, exists := m.users[user.ID]
	if !exists {
		return appErrors.NotFound("user not found")
	}
	existing.Username = user.Username
	existing.Email = user.Email
	existing.FirstName = user.FirstName
	existing.LastName = user.LastName
	existing.Phone = user.Phone
	existing.AlternatePhone = user.AlternatePhone
	existing.DateOfBirth = user.DateOfBirth
	existing.Gender = user.Gender
	existing.Bio = user.Bio
	existing.AvatarURL = user.AvatarURL
	existing.UpdatedAt = time.Now()
	return nil
}

func TestGetUser_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-123",
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	res, err := svc.GetUser(context.Background(), "user-123", "user-123")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.ID != "user-123" || res.Email != "test@example.com" {
		t.Errorf("unexpected user returned: %+v", res)
	}
}

func TestGetUser_NonExistent(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	_, err := svc.GetUser(context.Background(), "unknown", "unknown")
	if err == nil {
		t.Fatal("expected error for nonexistent user, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND code, got %s", appErr.Code)
	}
}

func TestGetUser_InactiveStatus(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-inactive",
		Email:     "inactive@example.com",
		FirstName: "Jane",
		LastName:  "Doe",
		Role:      "customer",
		Status:    "suspended",
	}
	repo.users[user.ID] = user

	_, err := svc.GetUser(context.Background(), "user-inactive", "user-inactive")
	if err == nil {
		t.Fatal("expected error for inactive status, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestGetUserByID_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-456",
		Email:     "byid@example.com",
		FirstName: "Alice",
		LastName:  "Smith",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	res, err := svc.GetUserByID(context.Background(), "user-456")
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if res.ID != "user-456" {
		t.Errorf("expected user-456, got %s", res.ID)
	}
}

func TestUpdateUser_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-789",
		Email:     "old@example.com",
		FirstName: "Bob",
		LastName:  "Marley",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	newEmail := "new@example.com"
	newFirst := "Robert"
	newPhone := "+1234567890"
	altPhone := "+19876543210"
	dob := "1990-05-15"
	gender := "Male"
	bio := "Tech enthusiast, music lover, and frequent GoCart shopper."
	avatar := "https://example.com/avatar.jpg"

	req := dto.UpdateUserRequest{
		Email:          &newEmail,
		FirstName:      &newFirst,
		Phone:          &newPhone,
		AlternatePhone: &altPhone,
		DateOfBirth:    &dob,
		Gender:         &gender,
		About:          &bio,
		AvatarURL:      &avatar,
	}

	updated, err := svc.UpdateUser(context.Background(), "user-789", "user-789", req)
	if err != nil {
		t.Fatalf("expected update success, got error: %v", err)
	}

	if updated.Email != "new@example.com" {
		t.Errorf("expected updated email new@example.com, got %s", updated.Email)
	}
	if updated.FirstName != "Robert" {
		t.Errorf("expected updated first_name Robert, got %s", updated.FirstName)
	}
	if updated.Phone == nil || *updated.Phone != "+1234567890" {
		t.Errorf("expected updated phone +1234567890, got %v", updated.Phone)
	}
	if updated.AlternatePhone == nil || *updated.AlternatePhone != "+19876543210" {
		t.Errorf("expected updated alt phone +19876543210, got %v", updated.AlternatePhone)
	}
	if updated.DateOfBirth == nil || updated.DateOfBirth.Format("2006-01-02") != "1990-05-15" {
		t.Errorf("expected updated dob 1990-05-15, got %v", updated.DateOfBirth)
	}
	if updated.Gender == nil || *updated.Gender != "Male" {
		t.Errorf("expected updated gender Male, got %v", updated.Gender)
	}
	if updated.Bio == nil || *updated.Bio != "Tech enthusiast, music lover, and frequent GoCart shopper." {
		t.Errorf("expected updated bio, got %v", updated.Bio)
	}
	if updated.AvatarURL == nil || *updated.AvatarURL != "https://example.com/avatar.jpg" {
		t.Errorf("expected updated avatar_url, got %v", updated.AvatarURL)
	}
}

func TestUpdateUser_CannotModifyAnotherUser(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "target-user",
		Email:     "target@example.com",
		FirstName: "Target",
		LastName:  "User",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	newFirst := "Hacked"
	req := dto.UpdateUserRequest{
		FirstName: &newFirst,
	}

	// authUserID ("attacker-user") != targetUserID ("target-user")
	_, err := svc.UpdateUser(context.Background(), "attacker-user", "target-user", req)
	if err == nil {
		t.Fatal("expected error when modifying another user's profile, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestUpdateUser_ProtectedFieldsPreserved(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-prot",
		Email:     "prot@example.com",
		FirstName: "Sam",
		LastName:  "Altman",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	newFirst := "Samuel"
	req := dto.UpdateUserRequest{
		FirstName: &newFirst,
	}

	updated, err := svc.UpdateUser(context.Background(), "user-prot", "user-prot", req)
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}

	// Protected fields must not change
	if updated.ID != "user-prot" {
		t.Errorf("user_id changed to %s", updated.ID)
	}
	if updated.Role != "customer" {
		t.Errorf("role changed to %s", updated.Role)
	}
	if updated.Status != "active" {
		t.Errorf("status changed to %s", updated.Status)
	}
}

func TestUpdateUser_InvalidInput(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-invalid",
		Email:     "valid@example.com",
		FirstName: "Val",
		LastName:  "Id",
		Role:      "customer",
		Status:    "active",
	}
	repo.users[user.ID] = user

	invalidEmail := "not-an-email"
	req := dto.UpdateUserRequest{
		Email: &invalidEmail,
	}

	_, err := svc.UpdateUser(context.Background(), "user-invalid", "user-invalid", req)
	if err == nil {
		t.Fatal("expected validation error for invalid email format, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeBadRequest {
		t.Errorf("expected BAD_REQUEST code, got %s", appErr.Code)
	}
}

func TestUpdateUser_EmailConflict(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user1 := &model.User{
		ID:     "user-1",
		Email:  "user1@example.com",
		Role:   "customer",
		Status: "active",
	}
	user2 := &model.User{
		ID:     "user-2",
		Email:  "user2@example.com",
		Role:   "customer",
		Status: "active",
	}
	repo.users[user1.ID] = user1
	repo.users[user2.ID] = user2

	existingEmail := "user2@example.com"
	req := dto.UpdateUserRequest{
		Email: &existingEmail,
	}

	_, err := svc.UpdateUser(context.Background(), "user-1", "user-1", req)
	if err == nil {
		t.Fatal("expected conflict error, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeConflict {
		t.Errorf("expected CONFLICT code, got %s", appErr.Code)
	}
}
