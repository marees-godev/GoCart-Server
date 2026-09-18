package service_test

import (
	"context"
	"testing"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"golang.org/x/crypto/bcrypt"
)

type mockAuthRepo struct {
	byEmail map[string]*model.AuthCredential
	byPhone map[string]*model.AuthCredential
	created []*model.AuthCredential
	outbox  []*outbox.Event
}

func newMockAuthRepo() *mockAuthRepo {
	return &mockAuthRepo{
		byEmail: make(map[string]*model.AuthCredential),
		byPhone: make(map[string]*model.AuthCredential),
	}
}

func (m *mockAuthRepo) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	if cred, ok := m.byEmail[email]; ok {
		return cred, nil
	}
	return nil, nil
}

func (m *mockAuthRepo) GetByPhone(ctx context.Context, phone string) (*model.AuthCredential, error) {
	if cred, ok := m.byPhone[phone]; ok {
		return cred, nil
	}
	return nil, nil
}

func (m *mockAuthRepo) CreateWithOutbox(ctx context.Context, cred *model.AuthCredential, outboxEvt *outbox.Event) error {
	m.byEmail[cred.Email] = cred
	if cred.Phone != nil {
		m.byPhone[*cred.Phone] = cred
	}
	m.created = append(m.created, cred)
	if outboxEvt != nil {
		m.outbox = append(m.outbox, outboxEvt)
	}
	return nil
}

func TestRegister_SuccessCustomer(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "  Customer@Example.com ",
		Phone:    "+919876543210",
		Password: "StrongPassword123!",
		Role:     "customer",
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Email != "customer@example.com" {
		t.Errorf("expected normalized email 'customer@example.com', got '%s'", resp.Email)
	}
	if resp.Role != "CUSTOMER" {
		t.Errorf("expected upper role 'CUSTOMER', got '%s'", resp.Role)
	}
	if resp.EmailVerified {
		t.Errorf("expected email_verified to be false")
	}

	if len(repo.created) != 1 {
		t.Fatalf("expected 1 credential record created, got %d", len(repo.created))
	}

	createdCred := repo.created[0]
	if createdCred.PasswordHash == req.Password {
		t.Errorf("password must not be saved in plaintext")
	}
	if err := bcrypt.CompareHashAndPassword([]byte(createdCred.PasswordHash), []byte(req.Password)); err != nil {
		t.Errorf("password hash verification failed: %v", err)
	}

	if len(repo.outbox) != 1 {
		t.Fatalf("expected 1 outbox event published, got %d", len(repo.outbox))
	}
	evt := repo.outbox[0]
	if evt.EventType != events.EventTypeUserRegistered {
		t.Errorf("expected event type '%s', got '%s'", events.EventTypeUserRegistered, evt.EventType)
	}
	if evt.AggregateType != "user" {
		t.Errorf("expected aggregate type 'user', got '%s'", evt.AggregateType)
	}
}

func TestRegister_SuccessMerchant(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "merchant@example.com",
		Phone:    "+919876543211",
		Password: "StrongPassword123!",
		Role:     "MERCHANT",
	}

	resp, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}

	if resp.Role != "MERCHANT" {
		t.Errorf("expected role 'MERCHANT', got '%s'", resp.Role)
	}
}

func TestRegister_RejectAdmin(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "admin@example.com",
		Password: "StrongPassword123!",
		Role:     "ADMIN",
	}

	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for ADMIN registration, got nil")
	}

	appErr, ok := err.(*errors.AppError)
	if !ok {
		t.Fatalf("expected AppError type, got %T", err)
	}
	if appErr.Code != errors.CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %s", appErr.Code)
	}

	if len(repo.created) != 0 {
		t.Errorf("failed registration should not persist credential record")
	}
	if len(repo.outbox) != 0 {
		t.Errorf("failed registration should not publish outbox event")
	}
}

func TestRegister_InvalidRole(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "user@example.com",
		Password: "StrongPassword123!",
		Role:     "SUPERUSER",
	}

	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for invalid role, got nil")
	}

	appErr := errors.AsAppError(err)
	if appErr.Code != errors.CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %s", appErr.Code)
	}
}

func TestRegister_DuplicateEmail(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "duplicate@example.com",
		Password: "StrongPassword123!",
		Role:     "CUSTOMER",
	}

	_, err := svc.Register(context.Background(), req)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	_, err = svc.Register(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error on duplicate email registration")
	}

	appErr := errors.AsAppError(err)
	if appErr.Code != errors.CodeConflict {
		t.Errorf("expected CodeConflict for duplicate email, got %s", appErr.Code)
	}
}

func TestRegister_DuplicatePhone(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req1 := dto.RegisterRequest{
		Email:    "user1@example.com",
		Phone:    "+919999999999",
		Password: "StrongPassword123!",
		Role:     "CUSTOMER",
	}

	req2 := dto.RegisterRequest{
		Email:    "user2@example.com",
		Phone:    "+919999999999",
		Password: "StrongPassword123!",
		Role:     "CUSTOMER",
	}

	_, err := svc.Register(context.Background(), req1)
	if err != nil {
		t.Fatalf("first registration failed: %v", err)
	}

	_, err = svc.Register(context.Background(), req2)
	if err == nil {
		t.Fatalf("expected error on duplicate phone registration")
	}

	appErr := errors.AsAppError(err)
	if appErr.Code != errors.CodeConflict {
		t.Errorf("expected CodeConflict for duplicate phone, got %s", appErr.Code)
	}
}

func TestRegister_ShortPassword(t *testing.T) {
	repo := newMockAuthRepo()
	svc := service.NewAuthService(repo)

	req := dto.RegisterRequest{
		Email:    "user@example.com",
		Password: "short",
		Role:     "CUSTOMER",
	}

	_, err := svc.Register(context.Background(), req)
	if err == nil {
		t.Fatalf("expected error for short password")
	}

	appErr := errors.AsAppError(err)
	if appErr.Code != errors.CodeBadRequest {
		t.Errorf("expected CodeBadRequest, got %s", appErr.Code)
	}
}
