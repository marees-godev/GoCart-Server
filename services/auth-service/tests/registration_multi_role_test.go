package tests

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/gofrs/uuid/v5"
	pb "github.com/marees-godev/GoCart-Server/contracts/protobuf/auth"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	userpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/user"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/config"
	authGRPC "github.com/marees-godev/GoCart-Server/services/auth-service/internal/handler/grpc"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/repository"
	"github.com/marees-godev/GoCart-Server/services/auth-service/internal/service"
	"google.golang.org/grpc"
)

type mockUserServiceClient struct {
	userpb.UserServiceClient
}

func (m *mockUserServiceClient) CreateUser(ctx context.Context, req *userpb.CreateUserRequest, opts ...grpc.CallOption) (*userpb.CreateUserResponse, error) {
	return &userpb.CreateUserResponse{
		User: &userpb.User{
			Id:        req.Id,
			Email:     req.Email,
			FirstName: req.FirstName,
			LastName:  req.LastName,
		},
	}, nil
}

type inMemoryAuthRepo struct {
	byEmailRole  map[string]*model.AuthCredential
	outboxEvents []*outbox.Event
}

func newInMemoryAuthRepo() *inMemoryAuthRepo {
	return &inMemoryAuthRepo{
		byEmailRole:  make(map[string]*model.AuthCredential),
		outboxEvents: make([]*outbox.Event, 0),
	}
}

func (r *inMemoryAuthRepo) GetByEmail(ctx context.Context, email string) (*model.AuthCredential, error) {
	for _, cred := range r.byEmailRole {
		if strings.EqualFold(cred.Email, email) {
			return cred, nil
		}
	}
	return nil, repository.ErrNotFound
}

func (r *inMemoryAuthRepo) GetByEmailAndRole(ctx context.Context, email string, role model.Role) (*model.AuthCredential, error) {
	key := strings.ToLower(email) + ":" + strings.ToUpper(role.String())
	cred, ok := r.byEmailRole[key]
	if !ok {
		return nil, repository.ErrNotFound
	}
	return cred, nil
}

func (r *inMemoryAuthRepo) CreateCredential(ctx context.Context, cred *model.AuthCredential) error {
	key := strings.ToLower(cred.Email) + ":" + strings.ToUpper(cred.Role.String())
	if _, exists := r.byEmailRole[key]; exists {
		return appErrors.Conflict("user with this email and role already exists")
	}
	r.byEmailRole[key] = cred
	return nil
}

func (r *inMemoryAuthRepo) DeleteCredential(ctx context.Context, id uuid.UUID) error {
	for k, cred := range r.byEmailRole {
		if cred.ID == id {
			delete(r.byEmailRole, k)
			return nil
		}
	}
	return nil
}

func (r *inMemoryAuthRepo) UpdateFailedLogin(ctx context.Context, id uuid.UUID, failedCount int, lockedUntil *time.Time) error {
	return nil
}

func (r *inMemoryAuthRepo) ResetFailedLogin(ctx context.Context, id uuid.UUID) error {
	return nil
}

func (r *inMemoryAuthRepo) CreateLoginSession(ctx context.Context, refreshToken *model.RefreshToken, evt *outbox.Event) error {
	if evt != nil {
		r.outboxEvents = append(r.outboxEvents, evt)
	}
	return nil
}

func (r *inMemoryAuthRepo) GetRefreshToken(ctx context.Context, tokenHash string) (*model.RefreshToken, error) {
	return nil, repository.ErrNotFound
}

func (r *inMemoryAuthRepo) RevokeRefreshToken(ctx context.Context, id uuid.UUID) error {
	return nil
}

type mockMerchantClient struct {
	merchantpb.MerchantServiceClient
	createdMerchants []*merchantpb.CreateMerchantRequest
}

func (m *mockMerchantClient) CreateMerchant(ctx context.Context, in *merchantpb.CreateMerchantRequest, opts ...grpc.CallOption) (*merchantpb.CreateMerchantResponse, error) {
	m.createdMerchants = append(m.createdMerchants, in)
	return &merchantpb.CreateMerchantResponse{
		Merchant: &merchantpb.Merchant{
			Id:            in.UserId,
			UserId:        in.UserId,
			BusinessName:  in.BusinessName,
			BusinessEmail: in.BusinessEmail,
			FirstName:     in.FirstName,
			LastName:      in.LastName,
		},
	}, nil
}

func TestGRPC_MultiRoleRegistrationAndUniqueness(t *testing.T) {
	repo := newInMemoryAuthRepo()
	cfg := &config.Config{
		JWT: config.JWTConfig{
			Secret:        "test-secret-key",
			ExpiryMinutes: 15,
		},
	}
	mockMerchant := &mockMerchantClient{}
	authSvc := service.NewAuthService(repo, cfg, nil, &mockUserServiceClient{}, mockMerchant)
	handler := authGRPC.NewAuthGRPCHandler(authSvc, nil)

	ctx := context.Background()
	testEmail := "john@example.com"
	testPassword := "SecurePassword123!"

	// 1. Register john@example.com as a CUSTOMER -> Success
	custResp, err := handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "John",
		LastName:   "Customer",
		IsMerchant: false,
	})
	if err != nil {
		t.Fatalf("expected customer registration to succeed, got error: %v", err)
	}
	if custResp.AccessToken == "" || custResp.UserId == "" {
		t.Fatalf("expected valid token and userId for customer, got %+v", custResp)
	}

	custCred, _ := repo.GetByEmailAndRole(ctx, testEmail, model.RoleCustomer)
	if custCred == nil || custCred.Role != model.RoleCustomer {
		t.Fatalf("expected CUSTOMER credential to be saved in repository")
	}

	// 2. Register john@example.com as a MERCHANT -> Success
	merchResp, err := handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "John",
		LastName:   "Merchant",
		IsMerchant: true,
	})
	if err != nil {
		t.Fatalf("expected merchant registration with same email to succeed, got error: %v", err)
	}
	if merchResp.AccessToken == "" || merchResp.UserId == "" {
		t.Fatalf("expected valid token and userId for merchant, got %+v", merchResp)
	}
	if custResp.UserId == merchResp.UserId {
		t.Errorf("expected distinct UserIds for customer and merchant, got identical: %s", custResp.UserId)
	}

	merchCred, _ := repo.GetByEmailAndRole(ctx, testEmail, model.RoleMerchant)
	if merchCred == nil || merchCred.Role != model.RoleMerchant {
		t.Fatalf("expected MERCHANT credential to be saved in repository")
	}

	// Verify merchant service CreateMerchant was called
	if len(mockMerchant.createdMerchants) != 1 {
		t.Fatalf("expected 1 call to merchant service CreateMerchant, got %d", len(mockMerchant.createdMerchants))
	}
	if mockMerchant.createdMerchants[0].UserId != merchResp.UserId {
		t.Errorf("expected merchant ID %s, got %s", merchResp.UserId, mockMerchant.createdMerchants[0].UserId)
	}
	if mockMerchant.createdMerchants[0].BusinessName != "John Merchant" {
		t.Errorf("expected business name John Merchant, got %s", mockMerchant.createdMerchants[0].BusinessName)
	}

	// 3. Registering john@example.com again as a CUSTOMER -> Fails with 409 Conflict
	_, err = handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "Duplicate",
		LastName:   "Customer",
		IsMerchant: false,
	})
	if err == nil {
		t.Fatalf("expected conflict error when re-registering as customer, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d (err: %v)", appErr.HTTPStatus, err)
	}

	// 4. Registering john@example.com again as a MERCHANT -> Fails with 409 Conflict
	_, err = handler.Register(ctx, &pb.RegisterRequest{
		Email:      testEmail,
		Password:   testPassword,
		FirstName:  "Duplicate",
		LastName:   "Merchant",
		IsMerchant: true,
	})
	if err == nil {
		t.Fatalf("expected conflict error when re-registering as merchant, got nil")
	}
	appErr = appErrors.AsAppError(err)
	if appErr.HTTPStatus != 409 {
		t.Errorf("expected HTTP 409 Conflict, got %d (err: %v)", appErr.HTTPStatus, err)
	}

	// 5. Verify outbox events: exactly 2 events created (1 customer, 1 merchant)
	if len(repo.outboxEvents) != 2 {
		t.Fatalf("expected 2 outbox events, got %d", len(repo.outboxEvents))
	}
	if repo.outboxEvents[0].Topic != "auth.user.registered" || repo.outboxEvents[0].EventType != "UserRegistered" {
		t.Errorf("expected first event to be auth.user.registered, got %s / %s", repo.outboxEvents[0].Topic, repo.outboxEvents[0].EventType)
	}
	if repo.outboxEvents[1].Topic != "auth.merchant.registered" || repo.outboxEvents[1].EventType != "MerchantRegistered" {
		t.Errorf("expected second event to be auth.merchant.registered, got %s / %s", repo.outboxEvents[1].Topic, repo.outboxEvents[1].EventType)
	}
}
