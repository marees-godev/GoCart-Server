package middleware_test

import (
	"context"
	"testing"

	"github.com/google/uuid"
	merchantpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/merchant"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/middleware"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type mockMerchantRepo struct {
	merchants map[uuid.UUID]*model.Merchant
}

func newMockMerchantRepo() *mockMerchantRepo {
	return &mockMerchantRepo{merchants: make(map[uuid.UUID]*model.Merchant)}
}

func (m *mockMerchantRepo) Create(ctx context.Context, merchant *model.Merchant) error {
	m.merchants[merchant.ID] = merchant
	return nil
}
func (m *mockMerchantRepo) GetByID(ctx context.Context, id uuid.UUID) (*model.Merchant, error) {
	merch, exists := m.merchants[id]
	if !exists {
		return nil, status.Error(codes.NotFound, "not found")
	}
	return merch, nil
}
func (m *mockMerchantRepo) GetByUserID(ctx context.Context, userID uuid.UUID) (*model.Merchant, error) {
	for _, merch := range m.merchants {
		if merch.UserID == userID {
			return merch, nil
		}
	}
	return nil, status.Error(codes.NotFound, "not found")
}
func (m *mockMerchantRepo) List(ctx context.Context, limit, offset int, status string) ([]*model.Merchant, int, error) {
	var list []*model.Merchant
	for _, merch := range m.merchants {
		list = append(list, merch)
	}
	return list, len(list), nil
}
func (m *mockMerchantRepo) Update(ctx context.Context, merchant *model.Merchant) error {
	m.merchants[merchant.ID] = merchant
	return nil
}
func (m *mockMerchantRepo) UpdateStatus(ctx context.Context, id uuid.UUID, s string, r string) (*model.Merchant, error) {
	merch, ok := m.merchants[id]
	if !ok {
		return nil, status.Error(codes.NotFound, "not found")
	}
	merch.Status = s
	merch.RejectionReason = r
	return merch, nil
}
func (m *mockMerchantRepo) Delete(ctx context.Context, id uuid.UUID) error {
	delete(m.merchants, id)
	return nil
}

func TestUnaryOwnershipInterceptor(t *testing.T) {
	repo := newMockMerchantRepo()
	interceptor := middleware.UnaryOwnershipInterceptor(repo)

	merchant1ID := uuid.New()
	user1ID := merchant1ID
	user2ID := uuid.New()

	merchant1 := &model.Merchant{
		ID:           merchant1ID,
		BusinessName: "Owner Store",
		Status:       "PENDING",
	}
	_ = repo.Create(context.Background(), merchant1)

	dummyHandler := func(ctx context.Context, req any) (any, error) {
		return "ok", nil
	}

	// Helper to attach metadata
	ctxForUser := func(uid, role string) context.Context {
		md := metadata.Pairs("x-user-id", uid, "x-user-role", role)
		return metadata.NewIncomingContext(context.Background(), md)
	}

	// 1. GetMerchant: Owner accessing own merchant -> ALLOWED
	info := &grpc.UnaryServerInfo{FullMethod: "/gocart.merchant.v1.MerchantService/GetMerchant"}
	_, err := interceptor(ctxForUser(user1ID.String(), "MERCHANT"), &merchantpb.GetMerchantRequest{Id: merchant1ID.String()}, info, dummyHandler)
	if err != nil {
		t.Errorf("expected owner access to succeed, got error: %v", err)
	}

	// 2. GetMerchant: Different merchant accessing user1's merchant -> FORBIDDEN
	_, err = interceptor(ctxForUser(user2ID.String(), "MERCHANT"), &merchantpb.GetMerchantRequest{Id: merchant1ID.String()}, info, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for non-owner, got: %v", err)
	}

	// 3. GetMerchant: Admin accessing merchant1 -> ALLOWED
	_, err = interceptor(ctxForUser("admin-uuid", "ADMIN"), &merchantpb.GetMerchantRequest{Id: merchant1ID.String()}, info, dummyHandler)
	if err != nil {
		t.Errorf("expected admin access to succeed, got error: %v", err)
	}

	// 4. GetMerchantByUserID: Owner accessing own userID -> ALLOWED
	infoByUser := &grpc.UnaryServerInfo{FullMethod: "/gocart.merchant.v1.MerchantService/GetMerchantByUserID"}
	_, err = interceptor(ctxForUser(user1ID.String(), "MERCHANT"), &merchantpb.GetMerchantByUserIDRequest{UserId: user1ID.String()}, infoByUser, dummyHandler)
	if err != nil {
		t.Errorf("expected owner access by user id to succeed, got error: %v", err)
	}

	// 5. GetMerchantByUserID: Different merchant accessing user1's merchant -> FORBIDDEN
	_, err = interceptor(ctxForUser(user2ID.String(), "MERCHANT"), &merchantpb.GetMerchantByUserIDRequest{UserId: user1ID.String()}, infoByUser, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for different merchant by user id, got: %v", err)
	}

	// 6. UpdateMerchant: Non-owner -> FORBIDDEN
	infoUpdate := &grpc.UnaryServerInfo{FullMethod: "/gocart.merchant.v1.MerchantService/UpdateMerchant"}
	_, err = interceptor(ctxForUser(user2ID.String(), "MERCHANT"), &merchantpb.UpdateMerchantRequest{Id: merchant1ID.String(), BusinessName: "Hack"}, infoUpdate, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied on UpdateMerchant for non-owner, got: %v", err)
	}

	// 7. UpdateMerchant: Owner -> ALLOWED
	_, err = interceptor(ctxForUser(user1ID.String(), "MERCHANT"), &merchantpb.UpdateMerchantRequest{Id: merchant1ID.String(), BusinessName: "New Name"}, infoUpdate, dummyHandler)
	if err != nil {
		t.Errorf("expected owner UpdateMerchant to succeed, got: %v", err)
	}

	// 8. DeleteMerchant: Non-owner -> FORBIDDEN
	infoDelete := &grpc.UnaryServerInfo{FullMethod: "/gocart.merchant.v1.MerchantService/DeleteMerchant"}
	_, err = interceptor(ctxForUser(user2ID.String(), "MERCHANT"), &merchantpb.DeleteMerchantRequest{Id: merchant1ID.String()}, infoDelete, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied on DeleteMerchant for non-owner, got: %v", err)
	}

	// 9. DeleteMerchant: Owner -> ALLOWED
	_, err = interceptor(ctxForUser(user1ID.String(), "MERCHANT"), &merchantpb.DeleteMerchantRequest{Id: merchant1ID.String()}, infoDelete, dummyHandler)
	if err != nil {
		t.Errorf("expected owner DeleteMerchant to succeed, got: %v", err)
	}

	// 10. UpdateMerchantStatus: Merchant -> FORBIDDEN (ADMIN only)
	infoStatus := &grpc.UnaryServerInfo{FullMethod: "/gocart.merchant.v1.MerchantService/UpdateMerchantStatus"}
	_, err = interceptor(ctxForUser(user1ID.String(), "MERCHANT"), &merchantpb.UpdateMerchantStatusRequest{Id: merchant1ID.String(), Status: "APPROVED"}, infoStatus, dummyHandler)
	if err == nil || status.Code(err) != codes.PermissionDenied {
		t.Errorf("expected PermissionDenied for merchant trying to update status, got: %v", err)
	}

	// 11. UpdateMerchantStatus: Admin -> ALLOWED
	_, err = interceptor(ctxForUser("admin-uuid", "ADMIN"), &merchantpb.UpdateMerchantStatusRequest{Id: merchant1ID.String(), Status: "APPROVED"}, infoStatus, dummyHandler)
	if err != nil {
		t.Errorf("expected admin UpdateMerchantStatus to succeed, got: %v", err)
	}
}
