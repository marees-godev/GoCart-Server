package resolvers

import (
	"context"
	"testing"

	ratingpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/rating"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/client"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	grpcPkg "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type mockRatingClientForResolvers struct {
	ratingpb.RatingServiceClient

	createResp *ratingpb.CreateRatingResponse
	createErr  error

	getResp *ratingpb.GetRatingResponse
	getErr  error

	getProductRatingsResp *ratingpb.GetProductRatingsResponse
	getProductRatingsErr  error

	updateResp *ratingpb.UpdateRatingResponse
	updateErr  error

	deleteResp *ratingpb.DeleteRatingResponse
	deleteErr  error

	flagRatingResp *ratingpb.FlagRatingResponse
	flagRatingErr  error

	getFlaggedRatingsResp *ratingpb.GetFlaggedRatingsResponse
	getFlaggedRatingsErr  error

	moderateRatingsResp *ratingpb.ModerateRatingResponse
	moderateRatingsErr error

	lastCreateReq            *ratingpb.CreateRatingRequest
	lastGetReq               *ratingpb.GetRatingRequest
	lastGetProductRatingsReq *ratingpb.GetProductRatingsRequest
	lastUpdateReq            *ratingpb.UpdateRatingRequest
	lastDeleteReq            *ratingpb.DeleteRatingRequest
	lastFlagRatingReq        *ratingpb.FlagRatingRequest
	lastGetFlaggedRatingsReq *ratingpb.GetFlaggedRatingsRequest
	lastModerateRatingsReq *ratingpb.ModerateRatingRequest
}

func (m *mockRatingClientForResolvers) CreateRating(ctx context.Context, in *ratingpb.CreateRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.CreateRatingResponse, error) {
	m.lastCreateReq = in
	if m.createErr != nil {
		return nil, m.createErr
	}
	return m.createResp, nil
}

func (m *mockRatingClientForResolvers) GetRating(ctx context.Context, in *ratingpb.GetRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.GetRatingResponse, error) {
	m.lastGetReq = in
	if m.getErr != nil {
		return nil, m.getErr
	}
	return m.getResp, nil
}

func (m *mockRatingClientForResolvers) GetProductRatings(ctx context.Context, in *ratingpb.GetProductRatingsRequest, opts ...grpcPkg.CallOption) (*ratingpb.GetProductRatingsResponse, error) {
	m.lastGetProductRatingsReq = in
	if m.getProductRatingsErr != nil {
		return nil, m.getProductRatingsErr
	}
	return m.getProductRatingsResp, nil
}

func (m *mockRatingClientForResolvers) UpdateRating(ctx context.Context, in *ratingpb.UpdateRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.UpdateRatingResponse, error) {
	m.lastUpdateReq = in
	if m.updateErr != nil {
		return nil, m.updateErr
	}
	return m.updateResp, nil
}

func (m *mockRatingClientForResolvers) DeleteRating(ctx context.Context, in *ratingpb.DeleteRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.DeleteRatingResponse, error) {
	m.lastDeleteReq = in
	if m.deleteErr != nil {
		return nil, m.deleteErr
	}
	return m.deleteResp, nil
}

func (m *mockRatingClientForResolvers) FlagRating(ctx context.Context, in *ratingpb.FlagRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.FlagRatingResponse, error) {
	m.lastFlagRatingReq = in
	if m.flagRatingErr != nil {
		return nil, m.flagRatingErr
	}
	return m.flagRatingResp, nil
}

func (m *mockRatingClientForResolvers) GetFlaggedRatings(ctx context.Context, in *ratingpb.GetFlaggedRatingsRequest, opts ...grpcPkg.CallOption) (*ratingpb.GetFlaggedRatingsResponse, error) {
	m.lastGetFlaggedRatingsReq = in
	if m.getFlaggedRatingsErr != nil {
		return nil, m.getFlaggedRatingsErr
	}
	return m.getFlaggedRatingsResp, nil
}

func (m *mockRatingClientForResolvers) ModerateRatings(ctx context.Context, in *ratingpb.ModerateRatingRequest, opts ...grpcPkg.CallOption) (*ratingpb.ModerateRatingResponse, error) {
	m.lastModerateRatingsReq = in 
	if m.moderateRatingsErr != nil {
		return nil, m.moderateRatingsErr
	}
	return m.moderateRatingsResp, nil
}

func newTestRatingResolver(mock *mockRatingClientForResolvers) (*mutationResolver, *queryResolver) {
	clientMgr := &client.ClientManager{
		RatingClient: mock,
	}
	r := &Resolver{
		ClientMgr: clientMgr,
	}
	return &mutationResolver{Resolver: r}, &queryResolver{Resolver: r}
}

func strPtr(s string) *string {
	return &s
}

func intPtr(i int) *int {
	return &i
}

func TestRatingResolvers_CreateRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)

	input := model.CreateRatingInput{
		ProductID:   "prod-1",
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		Rating:      5,
		Review:      strPtr("Great product!"),
	}

	_, err := mut.CreateRating(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED code, got %v", err)
	}
}

func TestRatingResolvers_CreateRating_NilClient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.CreateRatingInput{
		ProductID:   "prod-1",
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		Rating:      5,
	}

	_, err := r.CreateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_CreateRating_Success(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		createResp: &ratingpb.CreateRatingResponse{
			Rating: &ratingpb.Rating{
				Id:          "rate-101",
				UserId:      "user-123",
				ProductId:   "prod-1",
				OrderId:     "ord-1",
				OrderItemId: "item-1",
				Rating:      5,
				Review:      "Excellent quality!",
				CreatedAt:   "2026-10-28T10:00:00Z",
				UpdatedAt:   "2026-10-28T10:00:00Z",
			},
		},
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.CreateRatingInput{
		ProductID:   "prod-1",
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		Rating:      5,
		Review:      strPtr("Excellent quality!"),
	}

	res, err := mut.CreateRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "rate-101" || res.Rating != 5 || res.Review != "Excellent quality!" {
		t.Errorf("unexpected rating result: %+v", res)
	}
	if mock.lastCreateReq == nil {
		t.Fatal("expected CreateRating to be called on mock client")
	}
	if mock.lastCreateReq.UserId != "user-123" {
		t.Errorf("expected UserId user-123, got %s", mock.lastCreateReq.UserId)
	}
	if mock.lastCreateReq.ProductId != "prod-1" || mock.lastCreateReq.OrderId != "ord-1" || mock.lastCreateReq.OrderItemId != "item-1" {
		t.Errorf("unexpected IDs in request: %+v", mock.lastCreateReq)
	}
	if mock.lastCreateReq.Rating != 5 || mock.lastCreateReq.Review != "Excellent quality!" {
		t.Errorf("unexpected rating/review in request: %+v", mock.lastCreateReq)
	}
}

func TestRatingResolvers_CreateRating_Success_NilReview(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		createResp: &ratingpb.CreateRatingResponse{
			Rating: &ratingpb.Rating{
				Id:          "rate-102",
				UserId:      "user-123",
				ProductId:   "prod-1",
				OrderId:     "ord-1",
				OrderItemId: "item-1",
				Rating:      4,
				Review:      "",
			},
		},
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.CreateRatingInput{
		ProductID:   "prod-1",
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		Rating:      4,
		Review:      nil,
	}

	res, err := mut.CreateRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "rate-102" || res.Review != "" {
		t.Errorf("unexpected rating result: %+v", res)
	}
	if mock.lastCreateReq.Review != "" {
		t.Errorf("expected empty string for nil review, got %s", mock.lastCreateReq.Review)
	}
}

func TestRatingResolvers_CreateRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		createErr: status.Error(codes.FailedPrecondition, "cannot rate without purchase"),
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.CreateRatingInput{
		ProductID:   "prod-1",
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		Rating:      5,
	}

	_, err := mut.CreateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnprocessableEntity {
		t.Fatalf("expected UNPROCESSABLE_ENTITY, got %v", err)
	}
}

func TestRatingResolvers_UpdateRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)

	input := model.UpdateRatingInput{
		ID:     "rate-101",
		Rating: 4,
	}

	_, err := mut.UpdateRating(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func TestRatingResolvers_UpdateRating_NilClient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.UpdateRatingInput{
		ID:     "rate-101",
		Rating: 4,
	}

	_, err := r.UpdateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_UpdateRating_Success(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		updateResp: &ratingpb.UpdateRatingResponse{
			Rating: &ratingpb.Rating{
				Id:        "rate-101",
				UserId:    "user-123",
				ProductId: "prod-1",
				Rating:    4,
				Review:    "Updated review content",
			},
		},
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.UpdateRatingInput{
		ID:     "rate-101",
		Rating: 4,
		Review: strPtr("Updated review content"),
	}

	res, err := mut.UpdateRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "rate-101" || res.Rating != 4 || res.Review != "Updated review content" {
		t.Errorf("unexpected rating result: %+v", res)
	}
	if mock.lastUpdateReq == nil {
		t.Fatal("expected UpdateRating to be called")
	}
	if mock.lastUpdateReq.Id != "rate-101" || mock.lastUpdateReq.UserId != "user-123" {
		t.Errorf("unexpected Id or UserId in request: %+v", mock.lastUpdateReq)
	}
	if mock.lastUpdateReq.Rating != 4 || mock.lastUpdateReq.Review != "Updated review content" {
		t.Errorf("unexpected Rating or Review in request: %+v", mock.lastUpdateReq)
	}
}

func TestRatingResolvers_UpdateRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		updateErr: status.Error(codes.PermissionDenied, "user is not the author of this rating"),
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-456",
		Role:   auth.RoleUser,
	})

	input := model.UpdateRatingInput{
		ID:     "rate-101",
		Rating: 3,
	}

	_, err := mut.UpdateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeForbidden {
		t.Fatalf("expected FORBIDDEN, got %v", err)
	}
}

func TestRatingResolvers_DeleteRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)

	_, err := mut.DeleteRating(context.Background(), "rate-101")
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func TestRatingResolvers_DeleteRating_NilClient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	_, err := r.DeleteRating(ctx, "rate-101")
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_DeleteRating_Success(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		deleteResp: &ratingpb.DeleteRatingResponse{
			Success: true,
			Message: "Rating deleted successfully",
		},
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	res, err := mut.DeleteRating(ctx, "rate-101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.Message != "Rating deleted successfully" {
		t.Errorf("unexpected delete result: %+v", res)
	}
	if mock.lastDeleteReq == nil {
		t.Fatal("expected DeleteRating to be called")
	}
	if mock.lastDeleteReq.Id != "rate-101" || mock.lastDeleteReq.UserId != "user-123" {
		t.Errorf("unexpected DeleteRating request: %+v", mock.lastDeleteReq)
	}
}

func TestRatingResolvers_DeleteRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		deleteErr: status.Error(codes.NotFound, "rating not found"),
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	_, err := mut.DeleteRating(ctx, "rate-unknown")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestRatingResolvers_ProductRatings_NilClient(t *testing.T) {
	r := &queryResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	_, err := r.ProductRatings(context.Background(), "prod-1", nil, nil)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_ProductRatings_Success_DefaultPagination(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getProductRatingsResp: &ratingpb.GetProductRatingsResponse{
			Ratings: []*ratingpb.Rating{
				{
					Id:        "rate-1",
					ProductId: "prod-1",
					UserId:    "user-1",
					Rating:    5,
					Review:    "Superb!",
				},
				{
					Id:        "rate-2",
					ProductId: "prod-1",
					UserId:    "user-2",
					Rating:    4,
					Review:    "Good!",
				},
			},
			AverageRating: 4.5,
			Total:         2,
			Stars: &ratingpb.StarRating{
				FiveStar:  1,
				FourStar:  1,
				ThreeStar: 0,
				TwoStar:   0,
				OneStar:   0,
			},
		},
	}
	_, query := newTestRatingResolver(mock)

	res, err := query.ProductRatings(context.Background(), "prod-1", nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Total != 2 || res.AverageRating != 4.5 || len(res.Ratings) != 2 {
		t.Errorf("unexpected product ratings result: %+v", res)
	}

	if res.Stars == nil || res.Stars.FiveStar != 1 || res.Stars.FourStar != 1 {
		t.Errorf("unexpected stars distribution: %+v", res.Stars)
	}

	if mock.lastGetProductRatingsReq == nil {
		t.Fatal("expected GetProductRatings to be called")
	}
	if mock.lastGetProductRatingsReq.ProductId != "prod-1" {
		t.Errorf("expected ProductId prod-1, got %s", mock.lastGetProductRatingsReq.ProductId)
	}
	if mock.lastGetProductRatingsReq.Limit != 10 || mock.lastGetProductRatingsReq.Offset != 0 {
		t.Errorf("expected default pagination limit=10 offset=0, got limit=%d offset=%d",
			mock.lastGetProductRatingsReq.Limit, mock.lastGetProductRatingsReq.Offset)
	}
}

func TestRatingResolvers_ProductRatings_Success_CustomPagination(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getProductRatingsResp: &ratingpb.GetProductRatingsResponse{
			Ratings:       []*ratingpb.Rating{},
			AverageRating: 0.0,
			Total:         0,
		},
	}
	_, query := newTestRatingResolver(mock)

	_, err := query.ProductRatings(context.Background(), "prod-1", intPtr(25), intPtr(50))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if mock.lastGetProductRatingsReq.Limit != 25 || mock.lastGetProductRatingsReq.Offset != 50 {
		t.Errorf("expected custom pagination limit=25 offset=50, got limit=%d offset=%d",
			mock.lastGetProductRatingsReq.Limit, mock.lastGetProductRatingsReq.Offset)
	}
}

func TestRatingResolvers_ProductRatings_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getProductRatingsErr: status.Error(codes.Internal, "database failure"),
	}
	_, query := newTestRatingResolver(mock)

	_, err := query.ProductRatings(context.Background(), "prod-1", nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_GetRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	_, query := newTestRatingResolver(mock)

	_, err := query.GetRating(context.Background(), "rate-101")
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED, got %v", err)
	}
}

func TestRatingResolvers_GetRating_NilClient(t *testing.T) {
	r := &queryResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	_, err := r.GetRating(ctx, "rate-101")
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_GetRating_Success(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getResp: &ratingpb.GetRatingResponse{
			Rating: &ratingpb.Rating{
				Id:        "rate-101",
				ProductId: "prod-1",
				UserId:    "user-1",
				Rating:    5,
				Review:    "Nice!",
			},
		},
	}
	_, query := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	res, err := query.GetRating(ctx, "rate-101")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.ID != "rate-101" || res.Rating != 5 || res.Review != "Nice!" {
		t.Errorf("unexpected rating result: %+v", res)
	}
	if mock.lastGetReq == nil || mock.lastGetReq.Id != "rate-101" {
		t.Errorf("unexpected GetRating request: %+v", mock.lastGetReq)
	}
}

func TestRatingResolvers_GetRating_NotFound(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getErr: status.Error(codes.NotFound, "rating not found"),
	}
	_, query := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	_, err := query.GetRating(ctx, "rate-unknown")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}

func TestRatingResolvers_FlagRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)

	input := model.FlagRatingInput{
		RatingID: "rate-101",
		Category: model.FlagCategorySpam,
	}

	_, err := mut.FlagRating(context.Background(), input)
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED code, got %v", err)
	}
}

func TestRatingResolvers_FlagRating_NilClient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.FlagRatingInput{
		RatingID: "rate-101",
		Category: model.FlagCategorySpam,
	}

	_, err := r.FlagRating(ctx, input)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_FlagRating_InvalidCategory(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.FlagRatingInput{
		RatingID: "rate-101",
		Category: model.FlagCategory("INVALID_CATEGORY"),
	}

	_, err := mut.FlagRating(ctx, input)
	if err == nil {
		t.Fatal("expected error for invalid category, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", err)
	}
}

func TestRatingResolvers_FlagRating_Success(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		flagRatingResp: &ratingpb.FlagRatingResponse{
			Success: true,
			Message: "Rating flagged successfully",
		},
	}
	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.FlagRatingInput{
		RatingID: "rate-101",
		Category: model.FlagCategorySpam,
		Reason:   strPtr("Suspicious promotional link"),
	}

	res, err := mut.FlagRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !res.Success || res.Message != "Rating flagged successfully" {
		t.Errorf("unexpected f;ag rating response: %+v", res)
	}

	if mock.lastFlagRatingReq == nil {
		t.Fatal("expected FlagRating to be called")
	}

	if mock.lastFlagRatingReq.UserId != "user-123" || mock.lastFlagRatingReq.RatingId != "rate-101" {
		t.Errorf("unexpected UserId or RatingId in request: %+v", mock.lastFlagRatingReq)
	}

	if mock.lastFlagRatingReq.Category != ratingpb.FlagCategory_SPAM {
		t.Errorf("expected category SPAM, got %v", mock.lastFlagRatingReq.Category)
	}

	if mock.lastFlagRatingReq.Reason != "Suspicious promotional link" {
		t.Errorf("expected reason text, got %s", mock.lastFlagRatingReq.Reason)
	}

}

func TestRatingResolvers_FlagRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		flagRatingErr: status.Error(codes.AlreadyExists, "user has already submitted a flag for this review"),
	}

	mut, _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-123",
		Role:   auth.RoleUser,
	})

	input := model.FlagRatingInput{
		RatingID: "rate-101",
		Category: model.FlagCategorySpam,
	}

	_, err := mut.FlagRating(ctx, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeConflict {
		t.Fatalf("expected CONFLICT error, got %v", err)
	}
}


func TestRatingResolvers_GetFlaggedRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	_, query := newTestRatingResolver(mock)

	_, err := query.GetFlaggedRating(context.Background(), nil, nil)
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED code, got %v", err)
	}
}

func TestRatingResolvers_GetFlaggedRating_NilClient(t *testing.T) {
	r := &queryResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role:   auth.RoleAdmin,
	})

	_, err := r.GetFlaggedRating(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_GetFlaggedRating_Success(t *testing.T) {
	reasonStr := "Spam promotion"
	mock := &mockRatingClientForResolvers{
		getFlaggedRatingsResp: &ratingpb.GetFlaggedRatingsResponse{
			Items: []*ratingpb.FlaggedRatingItem{
				{
					Rating: &ratingpb.Rating{
						Id:        "rate-101",
						ProductId: "prod-202",
						UserId:    "user-303",
						Rating:    1,
						Review:    "Spam review here",
						IsFlagged: true,
						CreatedAt: "2026-11-03T10:00:00Z",
					},
					FlagCount: 1,
					Flags: []*ratingpb.RatingFlag{
						{
							Id:        "flag-1",
							UserId:    "flag-user-1",
							Category:  ratingpb.FlagCategory_SPAM,
							Reason:    reasonStr,
							CreatedAt: "2026-11-03T10:05:00Z",
						},
					},
				},
			},
			Total: 1,
		},
	}

	_, query := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role:   auth.RoleAdmin,
	})

	lim := 15
	off := 30
	res, err := query.GetFlaggedRating(ctx, &lim, &off)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if res.Total != 1 {
		t.Fatalf("expected Total 1, got %d", res.Total)
	}

	if len(res.Items) != 1 {
		t.Fatalf("expected 1 item, got %d", len(res.Items))
	}

	item := res.Items[0]
	if item.Rating == nil || item.Rating.ID != "rate-101" {
		t.Errorf("unexpected rating: %+v", item.Rating)
	}
	if item.FlagCount != 1 {
		t.Errorf("expected FlagCount 1, got %d", item.FlagCount)
	}
	if len(item.Flags) != 1 {
		t.Fatalf("expected 1 flag, got %d", len(item.Flags))
	}
	if item.Flags[0].Category != model.FlagCategorySpam {
		t.Errorf("expected SPAM category, got %s", item.Flags[0].Category)
	}
	if item.Flags[0].Reason == nil || *item.Flags[0].Reason != reasonStr {
		t.Errorf("expected reason %s, got %v", reasonStr, item.Flags[0].Reason)
	}

	if mock.lastGetFlaggedRatingsReq == nil {
		t.Fatal("expected GetFlaggedRatings to be called")
	}
	if mock.lastGetFlaggedRatingsReq.Limit != 15 || mock.lastGetFlaggedRatingsReq.Offset != 30 {
		t.Errorf("expected limit=15, offset=30, got limit=%d, offset=%d", mock.lastGetFlaggedRatingsReq.Limit, mock.lastGetFlaggedRatingsReq.Offset)
	}
	if mock.lastGetFlaggedRatingsReq.AdminId != "admin-101" {
		t.Errorf("expected AdminId admin-101, got %s", mock.lastGetFlaggedRatingsReq.AdminId)
	}
}

func TestRatingResolvers_GetFlaggedRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		getFlaggedRatingsErr: status.Error(codes.Internal, "internal database failure"),
	}

	_, query := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role:   auth.RoleAdmin,
	})

	_, err := query.GetFlaggedRating(ctx, nil, nil)
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
	
}

func TestRatingResolvers_ModerateRating_Unauthenticated(t *testing.T) {
	mock := &mockRatingClientForResolvers{}
	mut, _ := newTestRatingResolver(mock)
	input := model.ModerateRatingInput{
		RatingID: "rate-101",
		IsHidden: true,
	}

	_, err := mut.ModerateRating(context.Background(),input)
	if err == nil {
		t.Fatal("expected error for unauthenticated user, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeUnauthorized {
		t.Fatalf("expected UNAUTHORIZED code, got %v",err)
	}

}

func TestRatingResolvers_ModerateRating_Nilclient(t *testing.T) {
	r := &mutationResolver{
		Resolver: &Resolver{
			ClientMgr: nil,
		},
	}

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role: auth.RoleAdmin,
	})

	input := model.ModerateRatingInput{
		RatingID: "rate-101",
		IsHidden: true,
	}

	_, err := r.ModerateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error for nil client, got nil")
	}

	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeInternalError {
		t.Fatalf("expected INTERNAL_SERVER_ERROR, got %v", err)
	}
}

func TestRatingResolvers_ModerateRating_Success_Hide(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		moderateRatingsResp: &ratingpb.ModerateRatingResponse{
			Success: true,
			Message: "Rating hidden successfully",
		},
	}

	mut , _ := newTestRatingResolver(mock)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role: auth.RoleAdmin,
	})

	input := model.ModerateRatingInput{
		RatingID: "rate-101",
		IsHidden: true,
	}

	res,err := mut.ModerateRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error : %v",err)
	}

	if !res.Success || res.Message != "Rating hidden successfully" {
		t.Errorf("unexpected moderate response: %+v",res)
	}

	if mock.lastModerateRatingsReq == nil {
		t.Fatal("expected ModerateRatings to be called")
	}

	if mock.lastModerateRatingsReq.RatingId != "rate-101" {
		t.Errorf("expected RatingId rate-101, got %s",mock.lastModerateRatingsReq.RatingId)
	}

	if mock.lastModerateRatingsReq.AdminId != "admin-101" {
		t.Errorf("expected AdminId admin-101, got %s",mock.lastModerateRatingsReq.AdminId)
	}

	if !mock.lastModerateRatingsReq.IsHidden {
		t.Errorf("expected IsHidden true, got false")
	}
}

func TestRatingResolvers_ModerateRating_Success_Unhide(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		moderateRatingsResp: &ratingpb.ModerateRatingResponse{
			Success: true,
			Message: "Rating unhidden successfully",
		},
	}
	mut, _ := newTestRatingResolver(mock)
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role:   auth.RoleAdmin,
	})
	input := model.ModerateRatingInput{
		RatingID: "rate-101",
		IsHidden: false,
	}
	res, err := mut.ModerateRating(ctx, input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.Success || res.Message != "Rating unhidden successfully" {
		t.Errorf("unexpected moderate response: %+v", res)
	}
	if mock.lastModerateRatingsReq == nil {
		t.Fatal("expected ModerateRatings to be called")
	}
	if mock.lastModerateRatingsReq.IsHidden {
		t.Errorf("expected IsHidden false, got true")
	}
}

func TestRatingResolvers_ModerateRating_GRPCError(t *testing.T) {
	mock := &mockRatingClientForResolvers{
		moderateRatingsErr: status.Error(codes.NotFound, "rating not found"),
	}
	mut, _ := newTestRatingResolver(mock)
	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "admin-101",
		Role:   auth.RoleAdmin,
	})
	input := model.ModerateRatingInput{
		RatingID: "rate-unknown",
		IsHidden: true,
	}
	_, err := mut.ModerateRating(ctx, input)
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	appErr, ok := err.(*appErrors.AppError)
	if !ok || appErr.Code != appErrors.CodeNotFound {
		t.Fatalf("expected NOT_FOUND, got %v", err)
	}
}