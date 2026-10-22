package service

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
)

type mockDeliveryRepoForServiceTest struct {
	repository.DeliveryRepository
	getPartnerProfileFunc func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error)
	upsertPartnerProfileFunc func(ctx context.Context, profile *model.DeliveryPartnerProfile) error
}

func (m *mockDeliveryRepoForServiceTest) GetPartnerProfile(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
	if m.getPartnerProfileFunc != nil {
		return m.getPartnerProfileFunc(ctx, partnerID)
	}
	return nil, appErrors.NotFound("not found")
}

func (m *mockDeliveryRepoForServiceTest) CreateOrUpdatePartnerProfile(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
	if m.upsertPartnerProfileFunc != nil {
		return m.upsertPartnerProfileFunc(ctx, profile)
	}
	return nil
}

func TestUpdatePartnerProfile_ErrorHandling(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("invalid partner id format returns bad request", func(t *testing.T) {
		svc := NewDeliveryService(&mockDeliveryRepoForServiceTest{})
		_, err := svc.UpdatePartnerProfile(ctx, "invalid-uuid", nil, nil, nil)
		if err == nil {
			t.Fatal("expected error for invalid uuid, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
			t.Fatalf("expected bad request error, got %v", err)
		}
	})

	t.Run("infrastructure failure is returned directly and not masked as new profile", func(t *testing.T) {
		infraErr := appErrors.Internal(errors.New("db pool down"), "db pool connection timeout")
		upsertCalled := false

		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return nil, infraErr
			},
			upsertPartnerProfileFunc: func(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
				upsertCalled = true
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		company := "Fast Logistics"
		_, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &company, nil, nil)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, infraErr) && err != infraErr {
			t.Fatalf("expected infraErr %v, got %v", infraErr, err)
		}
		if upsertCalled {
			t.Fatal("expected CreateOrUpdatePartnerProfile NOT to be called on infrastructure error")
		}
	})

	t.Run("not found error initializes new profile and succeeds", func(t *testing.T) {
		var savedProfile *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return nil, appErrors.NotFound("partner profile not found")
			},
			upsertPartnerProfileFunc: func(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
				savedProfile = profile
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		company := "New Partner Corp"
		email := "partner@example.com"
		contact := "+1-555-0199"
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &company, &email, &contact)
		if err != nil {
			t.Fatalf("expected success, got error %v", err)
		}
		if res == nil || res.PartnerID != partnerUUID {
			t.Fatalf("expected partnerID %s, got %v", partnerUUID, res)
		}
		if savedProfile == nil || savedProfile.CompanyName != company {
			t.Fatalf("expected saved company %s, got %v", company, savedProfile)
		}
	})

	t.Run("existing profile is updated successfully", func(t *testing.T) {
		existing := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Old Name",
			Email:       "old@example.com",
			IsActive:    true,
			CreatedAt:   time.Now().Add(-24 * time.Hour),
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return existing, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		newCompany := "Updated Name"
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &newCompany, nil, nil)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if res.CompanyName != newCompany {
			t.Fatalf("expected %s, got %s", newCompany, res.CompanyName)
		}
		if res.Email != "old@example.com" {
			t.Fatalf("expected email to remain old@example.com, got %s", res.Email)
		}
	})

	t.Run("empty string clears optional field while nil leaves it unchanged", func(t *testing.T) {
		existing := &model.DeliveryPartnerProfile{
			ID:            uuid.New(),
			PartnerID:     partnerUUID,
			CompanyName:   "Old Company",
			Email:         "existing@example.com",
			ContactNumber: "+1-555-1234",
			IsActive:      true,
			CreatedAt:     time.Now().Add(-24 * time.Hour),
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return existing, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, profile *model.DeliveryPartnerProfile) error {
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		emptyCompany := ""
		emptyContact := ""
		// companyName and contactNumber are present as empty string; email is nil (omitted)
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &emptyCompany, nil, &emptyContact)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if res.CompanyName != "" {
			t.Fatalf("expected company name to be cleared to empty string, got %s", res.CompanyName)
		}
		if res.ContactNumber != "" {
			t.Fatalf("expected contact number to be cleared to empty string, got %s", res.ContactNumber)
		}
		if res.Email != "existing@example.com" {
			t.Fatalf("expected email to remain unchanged, got %s", res.Email)
		}
	})

	t.Run("clearing email to empty string is rejected with bad request", func(t *testing.T) {
		existing := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			Email:     "valid@example.com",
			IsActive:  true,
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return existing, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		emptyEmail := ""
		_, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), nil, &emptyEmail, nil)
		if err == nil {
			t.Fatal("expected bad request when email is empty, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
			t.Fatalf("expected bad request, got %v", err)
		}
	})
}
