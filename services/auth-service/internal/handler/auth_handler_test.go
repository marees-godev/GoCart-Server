package handler_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gofiber/fiber/v2"
	"github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
)

type mockRepo struct {
	byEmail map[string]*model.AuthCredential
	byPhone map[string]*model.AuthCredential
	created []*model.AuthCredential
}

func newMockRepo() *mockRepo {
	return &mockRepo{
		byEmail: make(map[string]*model.AuthCredential),
		byPhone: make(map[string]*model.AuthCredential),
	}
}

func (m *mockRepo) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	if cred, ok := m.byEmail[email]; ok {
		return cred, nil
	}
	return nil, nil
}

func (m *mockRepo) GetByPhone(ctx context.Context, phone string) (*model.AuthCredential, error) {
	if cred, ok := m.byPhone[phone]; ok {
		return cred, nil
	}
	return nil, nil
}

func (m *mockRepo) CreateWithOutbox(ctx context.Context, cred *model.AuthCredential, outboxEvt *outbox.Event) error {
	m.byEmail[cred.Email] = cred
	if cred.Phone != nil {
		m.byPhone[*cred.Phone] = cred
	}
	m.created = append(m.created, cred)
	return nil
}

func setupApp() (*fiber.App, *mockRepo) {
	repo := newMockRepo()
	svc := service.NewAuthService(repo)
	h := handler.NewAuthHandler(svc)

	app := fiber.New()
	h.RegisterRoutes(app)
	return app, repo
}

func TestHTTPRegister_CustomerSuccess(t *testing.T) {
	app, _ := setupApp()

	payload := dto.RegisterRequest{
		Email:    "user@example.com",
		Phone:    "+919876543210",
		Password: "StrongPassword123!",
		Role:     "CUSTOMER",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}

	var resDto dto.RegisterResponse
	if err := json.NewDecoder(resp.Body).Decode(&resDto); err != nil {
		t.Fatalf("failed to decode response: %v", err)
	}

	if resDto.Email != "user@example.com" {
		t.Errorf("expected email 'user@example.com', got '%s'", resDto.Email)
	}
	if resDto.Role != "CUSTOMER" {
		t.Errorf("expected role 'CUSTOMER', got '%s'", resDto.Role)
	}
}

func TestHTTPRegister_MerchantSuccess(t *testing.T) {
	app, _ := setupApp()

	payload := dto.RegisterRequest{
		Email:    "merchant@example.com",
		Password: "StrongPassword123!",
		Role:     "MERCHANT",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if resp.StatusCode != http.StatusCreated {
		t.Errorf("expected status 201 Created, got %d", resp.StatusCode)
	}
}

func TestHTTPRegister_AdminRejected(t *testing.T) {
	app, _ := setupApp()

	payload := dto.RegisterRequest{
		Email:    "admin@example.com",
		Password: "StrongPassword123!",
		Role:     "ADMIN",
	}
	body, _ := json.Marshal(payload)

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}

	var errResp errors.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}

	if errResp.Error.Code != errors.CodeBadRequest {
		t.Errorf("expected error code BAD_REQUEST, got '%s'", errResp.Error.Code)
	}
}

func TestHTTPRegister_DuplicateEmailConflict(t *testing.T) {
	app, _ := setupApp()

	payload := dto.RegisterRequest{
		Email:    "dup@example.com",
		Password: "StrongPassword123!",
		Role:     "CUSTOMER",
	}
	body, _ := json.Marshal(payload)

	// First request succeeds
	req1 := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req1.Header.Set("Content-Type", "application/json")
	resp1, _ := app.Test(req1)
	if resp1.StatusCode != http.StatusCreated {
		t.Fatalf("first request failed with status %d", resp1.StatusCode)
	}

	// Second request fails with conflict
	req2 := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader(body))
	req2.Header.Set("Content-Type", "application/json")
	resp2, _ := app.Test(req2)

	if resp2.StatusCode != http.StatusConflict {
		t.Errorf("expected status 409 Conflict, got %d", resp2.StatusCode)
	}

	var errResp errors.ErrorResponse
	if err := json.NewDecoder(resp2.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}

	if errResp.Error.Code != errors.CodeConflict {
		t.Errorf("expected error code CONFLICT, got '%s'", errResp.Error.Code)
	}
}

func TestHTTPRegister_InvalidPayloadFormat(t *testing.T) {
	app, _ := setupApp()

	req := httptest.NewRequest(http.MethodPost, "/auth/register", bytes.NewReader([]byte("{invalid-json")))
	req.Header.Set("Content-Type", "application/json")

	resp, err := app.Test(req)
	if err != nil {
		t.Fatalf("app.Test failed: %v", err)
	}

	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("expected status 400 Bad Request, got %d", resp.StatusCode)
	}

	var errResp errors.ErrorResponse
	if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
		t.Fatalf("failed to decode error envelope: %v", err)
	}

	if errResp.Error.Code != errors.CodeBadRequest {
		t.Errorf("expected error code BAD_REQUEST, got '%s'", errResp.Error.Code)
	}
}
