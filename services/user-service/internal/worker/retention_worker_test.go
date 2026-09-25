package worker_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/marees-godev/GoCart-Server/services/user-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/worker"
)

type mockUserService struct {
	processedCount atomic.Int32
}

func (m *mockUserService) CreateUser(ctx context.Context, req dto.CreateUserRequest) (*model.User, error) {
	return nil, nil
}
func (m *mockUserService) GetUser(ctx context.Context, authUserID, targetUserID string) (*model.User, error) {
	return nil, nil
}
func (m *mockUserService) GetUserByID(ctx context.Context, id string) (*model.User, error) {
	return nil, nil
}
func (m *mockUserService) UpdateUser(ctx context.Context, authUserID, targetUserID string, req dto.UpdateUserRequest) (*model.User, error) {
	return nil, nil
}
func (m *mockUserService) DeactivateUser(ctx context.Context, authUserID, targetUserID string, req dto.DeactivateUserRequest) (*dto.AccountActionResponse, error) {
	return nil, nil
}
func (m *mockUserService) ReactivateUser(ctx context.Context, authUserID, targetUserID string) (*dto.AccountActionResponse, error) {
	return nil, nil
}
func (m *mockUserService) DeleteUser(ctx context.Context, authUserID, targetUserID string, req dto.DeleteUserRequest) (*dto.AccountActionResponse, error) {
	return nil, nil
}
func (m *mockUserService) ProcessExpiredDeactivations(ctx context.Context, retentionPeriod time.Duration) (int, error) {
	m.processedCount.Add(1)
	return 1, nil
}

func TestRetentionWorker_Lifecycle(t *testing.T) {
	svc := &mockUserService{}
	w := worker.NewRetentionWorker(svc, 10*time.Millisecond, 30*24*time.Hour)

	ctx, cancel := context.WithCancel(context.Background())
	w.Start(ctx)

	time.Sleep(35 * time.Millisecond)
	cancel()

	select {
	case <-w.Done():
	case <-time.After(1 * time.Second):
		t.Fatal("worker did not shut down in time")
	}

	count := svc.processedCount.Load()
	if count < 2 {
		t.Errorf("expected at least 2 processed calls, got %d", count)
	}
}
