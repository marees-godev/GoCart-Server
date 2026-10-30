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

	lastCreateReq            *ratingpb.CreateRatingRequest
	lastGetReq               *ratingpb.GetRatingRequest
	lastGetProductRatingsReq *ratingpb.GetProductRatingsRequest
	lastUpdateReq            *ratingpb.UpdateRatingRequest
	lastDeleteReq            *ratingpb.DeleteRatingRequest
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
