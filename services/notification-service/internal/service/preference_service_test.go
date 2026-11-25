package service_test

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type mockPrefRepo struct {
	lastUserID string
}

func (m *mockPrefRepo) GetPreference(ctx context.Context, userID, channelID, category string) (*model.Preference, error) {
	return nil, nil
}

func (m *mockPrefRepo) UpsertPreference(ctx context.Context, pref *model.Preference) error {
	return nil
}

func (m *mockPrefRepo) GetPreferencesByUserID(ctx context.Context, userID string) ([]*model.Preference, error) {
	m.lastUserID = userID
	return []*model.Preference{
		{ID: uuid.NewString(), UserID: userID, Category: service.CategoryOrders, IsEnabled: true},
	}, nil
}

type mockChanRepo struct{}

func (m *mockChanRepo) GetByCode(ctx context.Context, code string) (*model.Channel, error) {
	return &model.Channel{ID: "channel-1", Code: code}, nil
}

func (m *mockChanRepo) GetByID(ctx context.Context, id string) (*model.Channel, error) {
	return &model.Channel{ID: id, Code: "EMAIL"}, nil
}

func TestEventToCategory(t *testing.T) {
	tests := []struct {
		eventType    string
		wantCategory string
		wantOk       bool
	}{
		{"OrderCreated", service.CategoryOrders, true},
		{"order.placed", service.CategoryOrders, true},
		{"DeliveryStatusUpdated", service.CategoryDeliveries, true},
		{"ReturnApproved", service.CategoryReturns, true},
		{"PaymentFailed", service.CategoryPayments, true},
		{"OrderPaymentFailed", service.CategoryPayments, true},
		{"UserRegistered", service.CategoryAccount, true},
		{"StoreApproved", service.CategoryAccount, true},
		{"MerchantSuspended", service.CategoryAccount, true},
		{"unknown.custom.event", "", false},
		{"", "", false},
	}

	for _, tc := range tests {
		t.Run(tc.eventType, func(t *testing.T) {
			cat, ok := service.EventToCategory(tc.eventType)
			if ok != tc.wantOk {
				t.Fatalf("EventToCategory(%q) ok = %v, want %v", tc.eventType, ok, tc.wantOk)
			}
			if cat != tc.wantCategory {
				t.Fatalf("EventToCategory(%q) category = %q, want %q", tc.eventType, cat, tc.wantCategory)
			}
		})
	}
}

func TestGetUserPreferences_TrimsWhitespace(t *testing.T) {
	validUID := uuid.NewString()
	rawInput := "   " + validUID + "   "

	repo := &mockPrefRepo{}
	svc := service.NewPreferenceService(&mockChanRepo{}, repo)

	_, err := svc.GetUserPreferences(context.Background(), rawInput)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if repo.lastUserID != strings.TrimSpace(rawInput) {
		t.Fatalf("expected repo called with trimmed UID %q, got %q", strings.TrimSpace(rawInput), repo.lastUserID)
	}
}

func TestCheckPreferenceAndSelectChannel_UnknownEventAllowsByDefault(t *testing.T) {
	repo := &mockPrefRepo{}
	svc := service.NewPreferenceService(&mockChanRepo{}, repo)

	ch, allowed, err := svc.CheckPreferenceAndSelectChannel(context.Background(), uuid.NewString(), "unknown_event_type", "EMAIL")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !allowed {
		t.Fatalf("expected unknown event to be allowed by default")
	}
	if ch == nil || ch.Code != "EMAIL" {
		t.Fatalf("expected EMAIL channel, got %v", ch)
	}
}
