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
	users     map[string]*model.User
	auditLogs []*model.UserAuditLog
}

func newMockRepo() *mockUserRepository {
	return &mockUserRepository{
		users:     make(map[string]*model.User),
		auditLogs: make([]*model.UserAuditLog, 0),
	}
}

func (m *mockUserRepository) CreateUser(ctx context.Context, user *model.User) error {
	if m.users == nil {
		m.users = make(map[string]*model.User)
	}
	m.users[user.ID] = user
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
	existing.PhoneNumber = user.PhoneNumber
	existing.AlternatePhone = user.AlternatePhone
	existing.DateOfBirth = user.DateOfBirth
	existing.Gender = user.Gender
	existing.Bio = user.Bio
	existing.AvatarURL = user.AvatarURL
	existing.UpdatedAt = time.Now()
	return nil
}

func (m *mockUserRepository) DeactivateUser(ctx context.Context, userID, performedBy string, reason *string) error {
	u, exists := m.users[userID]
	if !exists {
		return appErrors.NotFound("user not found")
	}
	if u.Status == "deleted" {
		return appErrors.Forbidden("cannot deactivate a deleted account")
	}
	if u.Status == "deactivated" {
		return nil
	}
	now := time.Now()
	u.Status = "deactivated"
	u.DeactivatedAt = &now
	u.UpdatedAt = now
	m.auditLogs = append(m.auditLogs, &model.UserAuditLog{
		ID:          "audit-" + userID,
		UserID:      userID,
		Action:      "DEACTIVATE",
		PerformedBy: performedBy,
		Reason:      reason,
		CreatedAt:   now,
	})
	return nil
}

func (m *mockUserRepository) ReactivateUser(ctx context.Context, userID, performedBy string) error {
	u, exists := m.users[userID]
	if !exists {
		return appErrors.NotFound("user not found")
	}
	if u.Status == "deleted" {
		return appErrors.Forbidden("cannot reactivate a deleted account")
	}
	if u.Status == "active" {
		return nil
	}
	if u.DeactivatedAt != nil && time.Since(*u.DeactivatedAt) > 30*24*time.Hour {
		reason := "30-day deactivation grace period expired"
		_ = m.DeleteUser(ctx, userID, "SYSTEM", &reason)
		return appErrors.Forbidden("account deactivation period of 30 days has expired; account has been permanently deleted")
	}
	now := time.Now()
	u.Status = "active"
	u.DeactivatedAt = nil
	u.UpdatedAt = now
	m.auditLogs = append(m.auditLogs, &model.UserAuditLog{
		ID:          "audit-" + userID,
		UserID:      userID,
		Action:      "REACTIVATE",
		PerformedBy: performedBy,
		Reason:      nil,
		CreatedAt:   now,
	})
	return nil
}

func (m *mockUserRepository) DeleteUser(ctx context.Context, userID, performedBy string, reason *string) error {
	u, exists := m.users[userID]
	if !exists {
		return appErrors.NotFound("user not found")
	}
	if u.Status == "deleted" {
		return nil
	}
	now := time.Now()
	u.Email = "deleted_" + userID + "@deleted.local"
	u.Username = nil
	u.FirstName = "Deleted"
	u.LastName = "User"
	u.PhoneNumber = nil
	u.AlternatePhone = nil
	u.DateOfBirth = nil
	u.Gender = nil
	u.Bio = nil
	u.AvatarURL = nil
	u.Status = "deleted"
	u.DeletedAt = &now
	u.UpdatedAt = now
	m.auditLogs = append(m.auditLogs, &model.UserAuditLog{
		ID:          "audit-" + userID,
		UserID:      userID,
		Action:      "DELETE",
		PerformedBy: performedBy,
		Reason:      reason,
		CreatedAt:   now,
	})
	return nil
}

func (m *mockUserRepository) GetExpiredDeactivatedUserIDs(ctx context.Context, cutoff time.Time) ([]string, error) {
	var ids []string
	for id, u := range m.users {
		if u.Status == "deactivated" && u.DeactivatedAt != nil && !u.DeactivatedAt.After(cutoff) {
			ids = append(ids, id)
		}
	}
	return ids, nil
}

func (m *mockUserRepository) GetUserAuditLogs(ctx context.Context, userID string) ([]*model.UserAuditLog, error) {
	var results []*model.UserAuditLog
	for _, l := range m.auditLogs {
		if l.UserID == userID {
			results = append(results, l)
		}
	}
	return results, nil
}

func TestGetUser_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-123",
		Email:     "test@example.com",
		FirstName: "John",
		LastName:  "Doe",
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
		PhoneNumber:    &newPhone,
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
	if updated.PhoneNumber == nil || *updated.PhoneNumber != "+1234567890" {
		t.Errorf("expected updated phone +1234567890, got %v", updated.PhoneNumber)
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
		Status: "active",
	}
	user2 := &model.User{
		ID:     "user-2",
		Email:  "user2@example.com",
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

func TestDeactivateUser_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-100",
		Email:     "user100@example.com",
		FirstName: "Active",
		LastName:  "User",
		Status:    "active",
	}
	repo.users[user.ID] = user

	reason := "Taking a break"
	res, err := svc.DeactivateUser(context.Background(), "user-100", "user-100", dto.DeactivateUserRequest{Reason: &reason})
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if !res.Success || res.Status != "deactivated" {
		t.Errorf("expected success with deactivated status, got %+v", res)
	}

	updated, _ := repo.GetByID(context.Background(), "user-100")
	if updated.Status != "deactivated" {
		t.Errorf("expected status deactivated, got %s", updated.Status)
	}

	logs, _ := repo.GetUserAuditLogs(context.Background(), "user-100")
	if len(logs) != 1 || logs[0].Action != "DEACTIVATE" {
		t.Errorf("expected 1 DEACTIVATE audit log, got %+v", logs)
	}
}

func TestDeactivateUser_Idempotent(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-101",
		Email:     "user101@example.com",
		FirstName: "Already",
		LastName:  "Deactivated",
		Status:    "deactivated",
	}
	repo.users[user.ID] = user

	res, err := svc.DeactivateUser(context.Background(), "user-101", "user-101", dto.DeactivateUserRequest{})
	if err != nil {
		t.Fatalf("expected no error on repeated deactivation, got %v", err)
	}
	if !res.Success || res.Status != "deactivated" {
		t.Errorf("expected success, got %+v", res)
	}
}

func TestDeactivateUser_CannotDeactivateDeleted(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:     "user-102",
		Email:  "deleted_user-102@deleted.local",
		Status: "deleted",
	}
	repo.users[user.ID] = user

	_, err := svc.DeactivateUser(context.Background(), "user-102", "user-102", dto.DeactivateUserRequest{})
	if err == nil {
		t.Fatal("expected error deactivating deleted user, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestDeactivateUser_ForbiddenOtherUser(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:     "user-103",
		Email:  "user103@example.com",
		Status: "active",
	}
	repo.users[user.ID] = user

	_, err := svc.DeactivateUser(context.Background(), "attacker-id", "user-103", dto.DeactivateUserRequest{})
	if err == nil {
		t.Fatal("expected forbidden error modifying other user, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErr.Code)
	}
}

func TestDeleteUser_Success(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	phone := "+1234567890"
	bio := "My bio"
	user := &model.User{
		ID:          "user-200",
		Email:       "user200@example.com",
		FirstName:   "John",
		LastName:    "Doe",
		PhoneNumber: &phone,
		Bio:         &bio,
		Status:      "active",
	}
	repo.users[user.ID] = user

	reason := "GDPR deletion request"
	res, err := svc.DeleteUser(context.Background(), "user-200", "user-200", dto.DeleteUserRequest{Reason: &reason})
	if err != nil {
		t.Fatalf("expected no error deleting user, got %v", err)
	}
	if !res.Success || res.Status != "deleted" {
		t.Errorf("expected success with deleted status, got %+v", res)
	}

	updated, _ := repo.GetByID(context.Background(), "user-200")
	if updated.Status != "deleted" {
		t.Errorf("expected status deleted, got %s", updated.Status)
	}
	if updated.Email != "deleted_user-200@deleted.local" {
		t.Errorf("expected email to be anonymized, got %s", updated.Email)
	}
	if updated.FirstName != "Deleted" || updated.LastName != "User" {
		t.Errorf("expected name to be anonymized, got %s %s", updated.FirstName, updated.LastName)
	}
	if updated.PhoneNumber != nil || updated.Bio != nil {
		t.Errorf("expected personal data cleared, got phone=%v bio=%v", updated.PhoneNumber, updated.Bio)
	}
	if updated.DeletedAt == nil {
		t.Errorf("expected DeletedAt to be set")
	}

	logs, _ := repo.GetUserAuditLogs(context.Background(), "user-200")
	if len(logs) != 1 || logs[0].Action != "DELETE" {
		t.Errorf("expected 1 DELETE audit log, got %+v", logs)
	}
}

func TestDeleteUser_Idempotent(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:     "user-201",
		Email:  "deleted_user-201@deleted.local",
		Status: "deleted",
	}
	repo.users[user.ID] = user

	res, err := svc.DeleteUser(context.Background(), "user-201", "user-201", dto.DeleteUserRequest{})
	if err != nil {
		t.Fatalf("expected no error on repeated deletion, got %v", err)
	}
	if !res.Success || res.Status != "deleted" {
		t.Errorf("expected success with deleted status, got %+v", res)
	}
}

func TestDeleteUser_FromDeactivated(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-202",
		Email:     "user202@example.com",
		FirstName: "Deactivated",
		LastName:  "User",
		Status:    "deactivated",
	}
	repo.users[user.ID] = user

	res, err := svc.DeleteUser(context.Background(), "user-202", "user-202", dto.DeleteUserRequest{})
	if err != nil {
		t.Fatalf("expected no error deleting deactivated user, got %v", err)
	}
	if !res.Success || res.Status != "deleted" {
		t.Errorf("expected success, got %+v", res)
	}

	updated, _ := repo.GetByID(context.Background(), "user-202")
	if updated.Status != "deleted" {
		t.Errorf("expected status deleted, got %s", updated.Status)
	}
}

func TestDeactivatedUser_CannotPerformOperations(t *testing.T) {
	repo := newMockRepo()
	svc := service.NewUserService(repo)

	user := &model.User{
		ID:        "user-300",
		Email:     "user300@example.com",
		FirstName: "Deactivated",
		LastName:  "Person",
		Status:    "deactivated",
	}
	repo.users[user.ID] = user

	// 1. GetUser fails
	_, err := svc.GetUser(context.Background(), "user-300", "user-300")
	if err == nil {
		t.Fatal("expected forbidden error for GetUser on deactivated user, got nil")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErrors.AsAppError(err).Code)
	}

	// 2. GetUserByID fails
	_, err = svc.GetUserByID(context.Background(), "user-300")
	if err == nil {
		t.Fatal("expected forbidden error for GetUserByID on deactivated user, got nil")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErrors.AsAppError(err).Code)
	}

	// 3. UpdateUser fails
	newFirst := "NewName"
	_, err = svc.UpdateUser(context.Background(), "user-300", "user-300", dto.UpdateUserRequest{FirstName: &newFirst})
	if err == nil {
		t.Fatal("expected forbidden error for UpdateUser on deactivated user, got nil")
	}
	if appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN code, got %s", appErrors.AsAppError(err).Code)
	}
}
