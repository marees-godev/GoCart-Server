package service

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/user-service/internal/provider"
)

type mockLocationRepo struct {
	data map[string]*model.Location
	saveErr error
}

func (m *mockLocationRepo) GetByPincode(ctx context.Context, pincode string) (*model.Location, error) {
	if loc, ok := m.data[pincode]; ok {
		return loc, nil
	}
	return nil, nil
}

func (m *mockLocationRepo) Save(ctx context.Context, loc *model.Location) error {
	if m.saveErr != nil {
		return m.saveErr
	}
	m.data[loc.Pincode] = loc
	return nil
}

type mockLocationProvider struct {
	lookupFunc func(ctx context.Context, pincode string) (*model.Location, error)
}

func (m *mockLocationProvider) LookupPincode(ctx context.Context, pincode string) (*model.Location, error) {
	if m.lookupFunc != nil {
		return m.lookupFunc(ctx, pincode)
	}
	return nil, provider.ErrLocationNotFound
}

func TestLocationService_Validation(t *testing.T) {
	svc := NewLocationService(&mockLocationRepo{}, &mockLocationProvider{})

	tests := []struct {
		name    string
		pincode string
		country string
		wantErr string
	}{
		{"Empty pincode", "", "IN", "pincode is required"},
		{"Whitespace only", "   ", "IN", "pincode is required"},
		{"Letters in pincode", "12345a", "IN", "must contain only numbers"},
		{"5 digits", "12345", "IN", "must be exactly 6 digits"},
		{"7 digits", "1234567", "IN", "must be exactly 6 digits"},
		{"Starts with 0", "012345", "IN", "cannot start with 0"},
		{"All same digits", "111111", "IN", "repeating digits are not allowed"},
		{"Delhi out of range", "110101", "IN", "Delhi PIN codes must be in the range 110001 - 110099"},
		{"Sequential pattern", "123456", "IN", "sequential patterns are not allowed"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := svc.GetLocationByPincode(context.Background(), tt.pincode, tt.country)
			if err == nil {
				t.Fatalf("expected error containing %q, got nil", tt.wantErr)
			}
			appErr := appErrors.AsAppError(err)
			if appErr.Code != appErrors.CodeBadRequest {
				t.Errorf("expected code BAD_REQUEST, got %s", appErr.Code)
			}
			if !strings.Contains(appErr.Message, tt.wantErr) {
				t.Errorf("expected error message to contain %q, got %q", tt.wantErr, appErr.Message)
			}
		})
	}
}

func TestLocationService_CacheHit(t *testing.T) {
	repo := &mockLocationRepo{
		data: map[string]*model.Location{
			"623517": {
				ID:       uuid.New(),
				Pincode:  "623517",
				Country:  "India",
				State:    "Tamil Nadu",
				District: "Ramanathapuram",
				PostOffices: []model.PostOffice{
					{Name: "Mandapam Camp", Pincode: "623517"},
				},
			},
		},
	}

	providerCalled := false
	prov := &mockLocationProvider{
		lookupFunc: func(ctx context.Context, pincode string) (*model.Location, error) {
			providerCalled = true
			return nil, nil
		},
	}

	svc := NewLocationService(repo, prov)
	loc, err := svc.GetLocationByPincode(context.Background(), "623517", "IN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if providerCalled {
		t.Errorf("provider should not have been called on cache hit")
	}
	if loc.Pincode != "623517" {
		t.Errorf("expected pincode 623517, got %s", loc.Pincode)
	}
}

func TestLocationService_CacheMiss_FetchesAndCaches(t *testing.T) {
	repo := &mockLocationRepo{
		data: make(map[string]*model.Location),
	}

	prov := &mockLocationProvider{
		lookupFunc: func(ctx context.Context, pincode string) (*model.Location, error) {
			return &model.Location{
				Pincode:  pincode,
				Country:  "India",
				State:    "Tamil Nadu",
				District: "Ramanathapuram",
				PostOffices: []model.PostOffice{
					{Name: "Mandapam Camp", Pincode: pincode},
					{Name: "Mandapam", Pincode: pincode},
				},
			}, nil
		},
	}

	svc := NewLocationService(repo, prov)
	loc, err := svc.GetLocationByPincode(context.Background(), "623517", "IN")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(loc.PostOffices) != 2 {
		t.Fatalf("expected 2 post offices, got %d", len(loc.PostOffices))
	}

	if cached, ok := repo.data["623517"]; !ok || len(cached.PostOffices) != 2 {
		t.Errorf("expected location to be saved in cache")
	}
}

func TestLocationService_ProviderNotFound(t *testing.T) {
	repo := &mockLocationRepo{data: make(map[string]*model.Location)}
	prov := &mockLocationProvider{
		lookupFunc: func(ctx context.Context, pincode string) (*model.Location, error) {
			return nil, provider.ErrLocationNotFound
		},
	}

	svc := NewLocationService(repo, prov)
	_, err := svc.GetLocationByPincode(context.Background(), "623999", "IN")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeNotFound {
		t.Errorf("expected code NOT_FOUND, got %s", appErr.Code)
	}
}

func TestLocationService_ProviderTimeout(t *testing.T) {
	repo := &mockLocationRepo{data: make(map[string]*model.Location)}
	prov := &mockLocationProvider{
		lookupFunc: func(ctx context.Context, pincode string) (*model.Location, error) {
			return nil, provider.ErrProviderTimeout
		},
	}

	svc := NewLocationService(repo, prov)
	_, err := svc.GetLocationByPincode(context.Background(), "623517", "IN")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeServiceUnavailable {
		t.Errorf("expected code SERVICE_UNAVAILABLE, got %s", appErr.Code)
	}
}

func TestLocationService_ProviderUnavailable(t *testing.T) {
	repo := &mockLocationRepo{data: make(map[string]*model.Location)}
	prov := &mockLocationProvider{
		lookupFunc: func(ctx context.Context, pincode string) (*model.Location, error) {
			return nil, provider.ErrProviderUnavailable
		},
	}

	svc := NewLocationService(repo, prov)
	_, err := svc.GetLocationByPincode(context.Background(), "623517", "IN")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	appErr := appErrors.AsAppError(err)
	if appErr.Code != appErrors.CodeServiceUnavailable {
		t.Errorf("expected code SERVICE_UNAVAILABLE, got %s", appErr.Code)
	}
}
