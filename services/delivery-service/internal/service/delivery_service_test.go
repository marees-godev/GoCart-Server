package service

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	gstinClientPkg "github.com/marees-godev/GoCart-Server/services/delivery-service/internal/client/gstin"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/repository"
)

type mockGSTINClient struct {
	disabled   bool
	verifyFunc func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error)
}

func (m *mockGSTINClient) IsEnabled() bool {
	return !m.disabled
}

func (m *mockGSTINClient) VerifyGSTIN(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
	if m.disabled {
		return nil, gstinClientPkg.ErrGSTINVerificationDisabled
	}
	if m.verifyFunc != nil {
		return m.verifyFunc(ctx, gstinStr)
	}
	return &gstinClientPkg.GSTINResponse{
		Success: true,
		GSTIN:   gstinStr,
		Data: &gstinClientPkg.GSTINData{
			GSTIN:     gstinStr,
			LegalName: "ACME LOGISTICS PRIVATE LIMITED",
			TradeName: "ACME LOGISTICS",
			Status:    "Active",
		},
	}, nil
}

type mockDeliveryRepoForServiceTest struct {
	repository.DeliveryRepository
	getByIDFunc                             func(ctx context.Context, id string) (*model.Delivery, error)
	updateStatusWithDetailsFunc             func(ctx context.Context, id string, status model.DeliveryStatus, est, act *time.Time, loc, notes string) (*model.Delivery, error)
	getPartnerProfileFunc                   func(ctx context.Context, partnerID uuid.UUID) (*model.DeliveryPartnerProfile, error)
	getPartnerProfileByGSTINFunc            func(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error)
	upsertPartnerProfileFunc                func(ctx context.Context, profile *model.DeliveryPartnerProfile) error
	listAppealedPartnersFunc                func(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error)
	listApprovedPartnersFunc                func(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error)
	softDeletePartnerProfileFunc            func(ctx context.Context, partnerID uuid.UUID, deletedAt time.Time, reason ...string) error
	softDeleteExpiredDeactivatedPartnersFunc func(ctx context.Context, cutoff time.Time) (int, error)
}

func (m *mockDeliveryRepoForServiceTest) GetByOrderID(ctx context.Context, orderID string) (*model.Delivery, error) {
	return nil, appErrors.NotFound("delivery not found")
}

func (m *mockDeliveryRepoForServiceTest) GetByID(ctx context.Context, id string) (*model.Delivery, error) {
	if m.getByIDFunc != nil {
		return m.getByIDFunc(ctx, id)
	}
	return nil, appErrors.NotFound("delivery not found")
}

func (m *mockDeliveryRepoForServiceTest) UpdateStatusWithDetails(ctx context.Context, id string, status model.DeliveryStatus, est, act *time.Time, loc, notes string) (*model.Delivery, error) {
	if m.updateStatusWithDetailsFunc != nil {
		return m.updateStatusWithDetailsFunc(ctx, id, status, est, act, loc, notes)
	}
	return nil, nil
}

func (m *mockDeliveryRepoForServiceTest) GetPartnerProfileByGSTIN(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error) {
	if m.getPartnerProfileByGSTINFunc != nil {
		return m.getPartnerProfileByGSTINFunc(ctx, gstin)
	}
	return nil, appErrors.NotFound("not found")
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

func (m *mockDeliveryRepoForServiceTest) ListAppealedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	if m.listAppealedPartnersFunc != nil {
		return m.listAppealedPartnersFunc(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockDeliveryRepoForServiceTest) ListApprovedPartners(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
	if m.listApprovedPartnersFunc != nil {
		return m.listApprovedPartnersFunc(ctx, limit, offset)
	}
	return nil, 0, nil
}

func (m *mockDeliveryRepoForServiceTest) SoftDeletePartnerProfile(ctx context.Context, partnerID uuid.UUID, deletedAt time.Time, reason ...string) error {
	if m.softDeletePartnerProfileFunc != nil {
		return m.softDeletePartnerProfileFunc(ctx, partnerID, deletedAt, reason...)
	}
	return nil
}

func (m *mockDeliveryRepoForServiceTest) SoftDeleteExpiredDeactivatedPartners(ctx context.Context, cutoff time.Time) (int, error) {
	if m.softDeleteExpiredDeactivatedPartnersFunc != nil {
		return m.softDeleteExpiredDeactivatedPartnersFunc(ctx, cutoff)
	}
	return 0, nil
}

func TestUpdatePartnerProfile_ErrorHandling(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("invalid partner id format returns bad request", func(t *testing.T) {
		svc := NewDeliveryService(&mockDeliveryRepoForServiceTest{})
		_, err := svc.UpdatePartnerProfile(ctx, "invalid-uuid", nil, nil, nil, nil)
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
		_, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &company, nil, nil, nil)
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
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &company, &email, &contact, nil)
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
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &newCompany, nil, nil, nil)
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
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &emptyCompany, nil, &emptyContact, nil)
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
		_, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), nil, &emptyEmail, nil, nil)
		if err == nil {
			t.Fatal("expected bad request when email is empty, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
			t.Fatalf("expected bad request, got %v", err)
		}
	})

	t.Run("caller matches profile partner_id or profile id", func(t *testing.T) {
		profileID := uuid.New()
		existing := &model.DeliveryPartnerProfile{
			ID:          profileID,
			PartnerID:   partnerUUID,
			CompanyName: "Old Name",
			Email:       "test@example.com",
			IsActive:    true,
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
		newComp := "New Company"

		// 1. Caller matches PartnerID
		res, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &newComp, nil, nil, nil, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected success when caller is partner_id, got %v", err)
		}
		if res.CompanyName != newComp {
			t.Errorf("expected %s, got %s", newComp, res.CompanyName)
		}

		// 2. Caller matches Profile ID
		_, err = svc.UpdatePartnerProfile(ctx, profileID.String(), &newComp, nil, nil, nil, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected success when caller is partner_id and target is profile ID, got %v", err)
		}

		// 3. Unauthorized caller rejected
		_, err = svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &newComp, nil, nil, nil, uuid.NewString())
		if err == nil {
			t.Fatal("expected forbidden error for unauthorized caller, got nil")
		}
	})
}

func TestPartnerApprovalLifecycle_AdminFlow(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("admin approves profile from PENDING status", func(t *testing.T) {
		gstin := "33AAACC1206D1ZN"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusPending,
			GSTIN:           &gstin,
			IsGSTINVerified: true,
			IsActive:        true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.ApprovePartner(ctx, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected approve to succeed, got %v", err)
		}
		if res.ApprovalStatus != model.ApprovalStatusApproved {
			t.Fatalf("expected status APPROVED, got %s", res.ApprovalStatus)
		}
		if saved.ApprovalStatus != model.ApprovalStatusApproved {
			t.Fatalf("expected saved status APPROVED, got %s", saved.ApprovalStatus)
		}
	})

	t.Run("admin rejects profile from PENDING status with reason", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			CompanyName:    "Acme Logistics",
			ApprovalStatus: model.ApprovalStatusPending,
			IsActive:       true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		reason := "Missing vehicle registration document"
		res, err := svc.RejectPartner(ctx, partnerUUID.String(), reason)
		if err != nil {
			t.Fatalf("expected reject to succeed, got %v", err)
		}
		if res.ApprovalStatus != model.ApprovalStatusRejected {
			t.Fatalf("expected status REJECTED, got %s", res.ApprovalStatus)
		}
		if res.RejectionReason == nil || *res.RejectionReason != reason {
			t.Fatalf("expected rejection reason '%s', got %v", reason, res.RejectionReason)
		}
		if saved.ApprovalStatus != model.ApprovalStatusRejected {
			t.Fatalf("expected saved status REJECTED, got %s", saved.ApprovalStatus)
		}
	})

	t.Run("admin rejection without reason fails", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			ApprovalStatus: model.ApprovalStatusPending,
			IsActive:       true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.RejectPartner(ctx, partnerUUID.String(), "   ")
		if err == nil {
			t.Fatal("expected error for empty rejection reason, got nil")
		}
	})

	t.Run("admin appeal re-evaluation APPEALED -> APPROVED", func(t *testing.T) {
		rejReason := "Old reason"
		appReason := "Updated documents submitted"
		gstin := "33AAACC1206D1ZN"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusAppealed,
			GSTIN:           &gstin,
			IsGSTINVerified: true,
			RejectionReason: &rejReason,
			AppealReason:    &appReason,
			IsActive:        true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.ApprovePartner(ctx, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected appeal re-evaluation approve to succeed, got %v", err)
		}
		if res.ApprovalStatus != model.ApprovalStatusApproved {
			t.Fatalf("expected status APPROVED, got %s", res.ApprovalStatus)
		}
		if res.RejectionReason != nil {
			t.Fatalf("expected rejection reason to be cleared, got %v", *res.RejectionReason)
		}
		if saved.ApprovalStatus != model.ApprovalStatusApproved {
			t.Fatalf("expected saved status APPROVED, got %s", saved.ApprovalStatus)
		}
	})

	t.Run("admin appeal re-evaluation APPEALED -> REJECTED", func(t *testing.T) {
		oldReason := "Old reason"
		appReason := "Updated documents submitted"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusAppealed,
			RejectionReason: &oldReason,
			AppealReason:    &appReason,
			IsActive:        true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		newReason := "Documents still invalid"
		res, err := svc.RejectPartner(ctx, partnerUUID.String(), newReason)
		if err != nil {
			t.Fatalf("expected appeal rejection to succeed, got %v", err)
		}
		if res.ApprovalStatus != model.ApprovalStatusRejected {
			t.Fatalf("expected status REJECTED, got %s", res.ApprovalStatus)
		}
		if res.RejectionReason == nil || *res.RejectionReason != newReason {
			t.Fatalf("expected new reason '%s', got %v", newReason, res.RejectionReason)
		}
		if saved.ApprovalStatus != model.ApprovalStatusRejected {
			t.Fatalf("expected saved status REJECTED, got %s", saved.ApprovalStatus)
		}
	})

	t.Run("admin approval or rejection from APPROVED state fails", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			ApprovalStatus: model.ApprovalStatusApproved,
			IsActive:       true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.ApprovePartner(ctx, partnerUUID.String())
		if err == nil {
			t.Fatal("expected error approving already APPROVED partner, got nil")
		}

		_, err = svc.RejectPartner(ctx, partnerUUID.String(), "rejecting approved")
		if err == nil {
			t.Fatal("expected error rejecting already APPROVED partner, got nil")
		}
	})
}

func TestPartnerAppeal_Validation(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("partner can appeal only from REJECTED state", func(t *testing.T) {
		rejReason := "Invalid insurance"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusRejected,
			RejectionReason: &rejReason,
			IsActive:        true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		appealReason := "Attached renewed insurance document"
		res, err := svc.AppealRejection(ctx, partnerUUID.String(), appealReason, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected appeal to succeed, got %v", err)
		}
		if res.ApprovalStatus != model.ApprovalStatusAppealed {
			t.Fatalf("expected status APPEALED, got %s", res.ApprovalStatus)
		}
		if res.AppealReason == nil || *res.AppealReason != appealReason {
			t.Fatalf("expected appeal reason '%s', got %v", appealReason, res.AppealReason)
		}
		if res.AppealedAt == nil {
			t.Fatal("expected appealed_at timestamp to be set")
		}
		if saved.ApprovalStatus != model.ApprovalStatusAppealed {
			t.Fatalf("expected saved status APPEALED, got %s", saved.ApprovalStatus)
		}
	})

	t.Run("partner appeal from PENDING state is rejected", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			ApprovalStatus: model.ApprovalStatusPending,
			IsActive:       true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.AppealRejection(ctx, partnerUUID.String(), "Premature appeal")
		if err == nil {
			t.Fatal("expected error appealing from PENDING state, got nil")
		}
	})

	t.Run("partner appeal from APPROVED state is rejected", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			ApprovalStatus: model.ApprovalStatusApproved,
			IsActive:       true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.AppealRejection(ctx, partnerUUID.String(), "Already approved")
		if err == nil {
			t.Fatal("expected error appealing from APPROVED state, got nil")
		}
	})

	t.Run("partner appeal rejected for non-owners", func(t *testing.T) {
		rejReason := "Invalid docs"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusRejected,
			RejectionReason: &rejReason,
			IsActive:        true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		otherPartnerID := uuid.New().String()
		_, err := svc.AppealRejection(ctx, partnerUUID.String(), "Appeal reason", otherPartnerID)
		if err == nil {
			t.Fatal("expected forbidden error for non-owner appeal, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeForbidden {
			t.Fatalf("expected forbidden error code, got %v", err)
		}
	})

	t.Run("partner appeal with empty reason is rejected", func(t *testing.T) {
		rejReason := "Invalid docs"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusRejected,
			RejectionReason: &rejReason,
			IsActive:        true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.AppealRejection(ctx, partnerUUID.String(), "    ")
		if err == nil {
			t.Fatal("expected error for empty appeal reason, got nil")
		}
	})
}

func TestPartnerDeletion_Ownership(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("owner-only deletion succeeds for owner", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			IsActive:  true,
		}
		softDeleteCalled := false
		var passedReason string
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			softDeletePartnerProfileFunc: func(ctx context.Context, pid uuid.UUID, dt time.Time, reason ...string) error {
				softDeleteCalled = true
				if len(reason) > 0 {
					passedReason = reason[0]
				}
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		err := svc.DeletePartnerProfile(ctx, partnerUUID.String(), "Business retired", partnerUUID.String())
		if err != nil {
			t.Fatalf("expected deletion to succeed, got %v", err)
		}
		if !softDeleteCalled {
			t.Fatal("expected SoftDeletePartnerProfile to be invoked")
		}
		if passedReason != "Business retired" {
			t.Fatalf("expected deletion reason 'Business retired', got %s", passedReason)
		}
	})

	t.Run("deletion forbidden for non-owners", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			IsActive:  true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		intruderID := uuid.New().String()
		err := svc.DeletePartnerProfile(ctx, partnerUUID.String(), "Business retired", intruderID)
		if err == nil {
			t.Fatal("expected forbidden error for non-owner deletion, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeForbidden {
			t.Fatalf("expected forbidden code, got %v", err)
		}
	})

	t.Run("deletion of already soft-deleted profile returns not found", func(t *testing.T) {
		deletedAt := time.Now().UTC()
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			DeletedAt: &deletedAt,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		err := svc.DeletePartnerProfile(ctx, partnerUUID.String(), "Business retired", partnerUUID.String())
		if err == nil {
			t.Fatal("expected not found error, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeNotFound {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("get profile of soft-deleted profile returns not found", func(t *testing.T) {
		deletedAt := time.Now().UTC()
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			DeletedAt: &deletedAt,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.GetPartnerProfile(ctx, partnerUUID.String())
		if err == nil {
			t.Fatal("expected not found error for deleted profile, got nil")
		}
		appErr := appErrors.AsAppError(err)
		if appErr == nil || appErr.Code != appErrors.CodeNotFound {
			t.Fatalf("expected not found error, got %v", err)
		}
	})
}

func TestPartnerActivation_And_RetentionWorker(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("deactivation sets is_active false and records deactivated_at", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			IsActive:  true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.DeactivatePartner(ctx, partnerUUID.String(), "Business hiatus")
		if err != nil {
			t.Fatalf("expected deactivation to succeed, got %v", err)
		}
		if res.IsActive {
			t.Fatal("expected is_active to be false")
		}
		if res.DeactivatedAt == nil {
			t.Fatal("expected deactivated_at to be set")
		}
		if res.DeactivationReason == nil || *res.DeactivationReason != "Business hiatus" {
			t.Fatalf("expected deactivation_reason 'Business hiatus', got %v", res.DeactivationReason)
		}
		if saved.IsActive || saved.DeactivatedAt == nil || saved.DeactivationReason == nil {
			t.Fatal("expected saved profile to reflect deactivation")
		}
	})

	t.Run("reactivation clears deactivated_at and sets is_active true", func(t *testing.T) {
		deactivatedAt := time.Now().UTC().Add(-5 * time.Minute)
		profile := &model.DeliveryPartnerProfile{
			ID:            uuid.New(),
			PartnerID:     partnerUUID,
			IsActive:      false,
			ApprovalStatus:  model.ApprovalStatusApproved,
			IsGSTINVerified: true,
			DeactivatedAt: &deactivatedAt,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.ActivatePartner(ctx, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected activation to succeed, got %v", err)
		}
		if !res.IsActive {
			t.Fatal("expected is_active to be true")
		}
		if res.DeactivatedAt != nil {
			t.Fatalf("expected deactivated_at to be cleared, got %v", res.DeactivatedAt)
		}
		if !saved.IsActive || saved.DeactivatedAt != nil {
			t.Fatal("expected saved profile to reflect activation")
		}
	})

	t.Run("ProcessExpiredDeactivations calculates 10-minute cutoff and soft-deletes", func(t *testing.T) {
		var passedCutoff time.Time
		mockRepo := &mockDeliveryRepoForServiceTest{
			softDeleteExpiredDeactivatedPartnersFunc: func(ctx context.Context, cutoff time.Time) (int, error) {
				passedCutoff = cutoff
				return 3, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		count, err := svc.ProcessExpiredDeactivations(ctx, 10*time.Minute)
		if err != nil {
			t.Fatalf("expected ProcessExpiredDeactivations to succeed, got %v", err)
		}
		if count != 3 {
			t.Fatalf("expected 3 processed profiles, got %d", count)
		}
		expectedCutoff := time.Now().UTC().Add(-10 * time.Minute)
		diff := expectedCutoff.Sub(passedCutoff)
		if diff < -2*time.Second || diff > 2*time.Second {
			t.Fatalf("expected cutoff ~ %v, got %v", expectedCutoff, passedCutoff)
		}
	})

	t.Run("ProcessExpiredDeactivations defaults to 30-day cutoff when period <= 0", func(t *testing.T) {
		var passedCutoff time.Time
		mockRepo := &mockDeliveryRepoForServiceTest{
			softDeleteExpiredDeactivatedPartnersFunc: func(ctx context.Context, cutoff time.Time) (int, error) {
				passedCutoff = cutoff
				return 2, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		count, err := svc.ProcessExpiredDeactivations(ctx, 0)
		if err != nil {
			t.Fatalf("expected ProcessExpiredDeactivations to succeed, got %v", err)
		}
		if count != 2 {
			t.Fatalf("expected 2 processed profiles, got %d", count)
		}
		expectedCutoff := time.Now().UTC().Add(-30 * 24 * time.Hour)
		diff := expectedCutoff.Sub(passedCutoff)
		if diff < -2*time.Second || diff > 2*time.Second {
			t.Fatalf("expected cutoff ~ %v, got %v", expectedCutoff, passedCutoff)
		}
	})
}

func TestListApprovedPartners_QueryExclusivity(t *testing.T) {
	ctx := context.Background()

	t.Run("returns only active, approved, and non-deleted partners", func(t *testing.T) {
		approvedPartners := []*model.DeliveryPartnerProfile{
			{
				ID:             uuid.New(),
				PartnerID:      uuid.New(),
				CompanyName:    "Express Fast Logistics",
				IsActive:       true,
				ApprovalStatus: model.ApprovalStatusApproved,
				DeletedAt:      nil,
			},
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			listApprovedPartnersFunc: func(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
				return approvedPartners, 1, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		list, total, err := svc.ListApprovedPartners(ctx, 10, 0)
		if err != nil {
			t.Fatalf("expected ListApprovedPartners to succeed, got %v", err)
		}
		if total != 1 || len(list) != 1 {
			t.Fatalf("expected 1 partner, got %d (total %d)", len(list), total)
		}
		if list[0].ApprovalStatus != model.ApprovalStatusApproved || !list[0].IsActive || list[0].DeletedAt != nil {
			t.Fatalf("partner does not satisfy exclusivity criteria: %+v", list[0])
		}
	})
}

func TestGSTIN_VerificationAndApproval(t *testing.T) {
	partnerUUID := uuid.New()
	ctx := context.Background()

	t.Run("VerifyGSTIN with valid format updates partner profile and sets verified", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:             uuid.New(),
			PartnerID:      partnerUUID,
			CompanyName:    "Acme Logistics",
			ApprovalStatus: model.ApprovalStatusPending,
			IsActive:       true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		validGSTIN := "33AAACC1206D1ZN"
		res, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), validGSTIN, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected VerifyGSTIN to succeed, got %v", err)
		}
		if res.GSTIN == nil || *res.GSTIN != validGSTIN {
			t.Fatalf("expected GSTIN %s, got %v", validGSTIN, res.GSTIN)
		}
		if !res.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
		if saved == nil || !saved.IsGSTINVerified {
			t.Fatal("expected saved profile to have is_gstin_verified true")
		}
	})

	t.Run("VerifyGSTIN with partnerID only uses stored GSTIN and succeeds", func(t *testing.T) {
		validGSTIN := "33AAACC1206D1ZN"
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "partner@test.com",
			GSTIN:       &validGSTIN,
			IsActive:    true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}

		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		res, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "")
		if err != nil {
			t.Fatalf("expected VerifyGSTIN with partnerID only to succeed, got %v", err)
		}
		if res.GSTIN == nil || *res.GSTIN != validGSTIN {
			t.Fatalf("expected GSTIN %s, got %v", validGSTIN, res.GSTIN)
		}
		if !res.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
		if saved == nil || !saved.IsGSTINVerified {
			t.Fatal("expected saved profile to have is_gstin_verified true")
		}
	})

	t.Run("VerifyGSTIN with invalid format fails", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "INVALID_GST", partnerUUID.String())
		if err == nil {
			t.Fatal("expected error for invalid GSTIN format, got nil")
		}
	})

	t.Run("VerifyGSTIN by admin caller succeeds", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "admin-verified@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		res, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err != nil {
			t.Fatalf("expected admin verify GSTIN to succeed, got %v", err)
		}
		if !res.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
	})

	t.Run("VerifyGSTIN fails when company name is empty", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "   ",
			Email:       "no-company@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err == nil {
			t.Fatal("expected error when company name is empty, got nil")
		}
		if !strings.Contains(err.Error(), "company_name is missing in delivery partner table") {
			t.Fatalf("expected error to mention missing company_name in delivery partner table, got %v", err)
		}
	})

	t.Run("VerifyGSTIN fails when GSTIN is not active", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "inactive@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return &gstinClientPkg.GSTINResponse{
					Success: true,
					GSTIN:   gstinStr,
					Data: &gstinClientPkg.GSTINData{
						GSTIN:     gstinStr,
						LegalName: "ACME LOGISTICS PRIVATE LIMITED",
						Status:    "Inactive",
					},
				}, nil
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err == nil {
			t.Fatal("expected error when GSTIN is inactive, got nil")
		}
		if !strings.Contains(err.Error(), "is not active") {
			t.Fatalf("expected error to state GSTIN is not active, got %v", err)
		}
	})

	t.Run("VerifyGSTIN fails when GSTIN is blocked", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "blocked@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return &gstinClientPkg.GSTINResponse{
					Success: true,
					GSTIN:   gstinStr,
					Data: &gstinClientPkg.GSTINData{
						GSTIN:       gstinStr,
						LegalName:   "ACME LOGISTICS PRIVATE LIMITED",
						Status:      "Active",
						BlockStatus: "Blocked",
					},
				}, nil
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err == nil {
			t.Fatal("expected error when GSTIN is blocked, got nil")
		}
		if !strings.Contains(err.Error(), "is blocked") {
			t.Fatalf("expected error to state GSTIN is blocked, got %v", err)
		}
	})

	t.Run("VerifyGSTIN fails with invalid credential on API 401 error", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "auth-fail@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return nil, errors.New("invalid credential")
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err == nil {
			t.Fatal("expected error for invalid credential, got nil")
		}
		if !strings.Contains(strings.ToLower(err.Error()), "invalid credential") {
			t.Fatalf("expected error message to contain 'invalid credential', got %v", err)
		}
	})

	t.Run("VerifyGSTIN fails with specific error when GSTIN is not found in GST database", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "not-found@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return nil, errors.New("GSTIN verification failed: GSTIN not found in GST database")
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33CSSPD6362E1Z8", "")
		if err == nil {
			t.Fatal("expected error for not found GSTIN, got nil")
		}
		if !strings.Contains(err.Error(), "invalid credential: GSTIN '33CSSPD6362E1Z8' is not found in GST database") {
			t.Fatalf("expected specific GSTIN not found error, got %v", err)
		}
	})

	t.Run("VerifyGSTIN by delivery partner caller succeeds", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "partner@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return &gstinClientPkg.GSTINResponse{
					Success: true,
					GSTIN:   gstinStr,
					Data: &gstinClientPkg.GSTINData{
						GSTIN:     gstinStr,
						LegalName: "ACME LOGISTICS PRIVATE LIMITED",
						Status:    "Active",
					},
				}, nil
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		res, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", partnerUUID.String())
		if err != nil {
			t.Fatalf("expected partner verify GSTIN to succeed, got %v", err)
		}
		if !res.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
	})

	t.Run("VerifyGSTIN fails when unauthorized non-owner caller calls it", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:        uuid.New(),
			PartnerID: partnerUUID,
			Email:     "partner@test.com",
			IsActive:  true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		svc := NewDeliveryService(mockRepo)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", uuid.New().String())
		if err == nil {
			t.Fatal("expected forbidden error for non-owner caller, got nil")
		}
	})

	t.Run("VerifyGSTIN fails when company name does not match external API", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Mismatched Express Inc",
			Email:       "partner@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			verifyFunc: func(ctx context.Context, gstinStr string) (*gstinClientPkg.GSTINResponse, error) {
				return &gstinClientPkg.GSTINResponse{
					Success: true,
					GSTIN:   gstinStr,
					Data: &gstinClientPkg.GSTINData{
						GSTIN:     gstinStr,
						LegalName: "CENTRAL WAREHOUSING CORPORATION",
						Status:    "Active",
					},
				}, nil
			},
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", partnerUUID.String())
		if err == nil {
			t.Fatal("expected error when company name does not match GSTIN details, got nil")
		}
		if !strings.Contains(err.Error(), "company_name 'Mismatched Express Inc' in delivery partner table does not match GSTIN registered name 'CENTRAL WAREHOUSING CORPORATION'") {
			t.Fatalf("expected error to specifically mention delivery partner table company name and registered name, got %v", err)
		}
		if profile.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to remain false after mismatch")
		}
	})

	t.Run("VerifyGSTIN fails when GSTIN verification is disabled and preserves unverified status", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			CompanyName:     "Acme Logistics",
			Email:           "partner@test.com",
			IsActive:        false,
			ApprovalStatus:  model.ApprovalStatusPending,
			IsGSTINVerified: false,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				return nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			disabled: true,
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", partnerUUID.String())
		if err == nil {
			t.Fatal("expected error when GSTIN verification is disabled, got nil")
		}
		if !strings.Contains(err.Error(), "GSTIN verification is disabled") {
			t.Fatalf("expected error to mention disabled GSTIN verification, got %v", err)
		}
		if profile.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to remain false when disabled")
		}
		if profile.ApprovalStatus != model.ApprovalStatusPending {
			t.Fatalf("expected approval status to remain pending, got %s", profile.ApprovalStatus)
		}
	})

	t.Run("VerifyGSTIN fails when GSTIN verification is disabled even for admin caller", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			CompanyName:     "Acme Logistics",
			Email:           "partner@test.com",
			IsActive:        false,
			ApprovalStatus:  model.ApprovalStatusPending,
			IsGSTINVerified: false,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				return nil
			},
		}
		mockGSTIN := &mockGSTINClient{
			disabled: true,
		}
		svc := NewDeliveryService(mockRepo, mockGSTIN)
		_, err := svc.VerifyGSTIN(ctx, partnerUUID.String(), "33AAACC1206D1ZN", "")
		if err == nil {
			t.Fatal("expected error when GSTIN verification is disabled, got nil")
		}
		if !strings.Contains(err.Error(), "GSTIN verification is disabled") {
			t.Fatalf("expected error to mention disabled GSTIN verification, got %v", err)
		}
		if profile.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to remain false")
		}
	})

	t.Run("UpdatePartnerProfile leaves is_gstin_verified false until VerifyGSTIN", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			CompanyName:     "Acme Logistics",
			Email:           "partner@test.com",
			IsActive:        true,
			IsGSTINVerified: false,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		svc := NewDeliveryService(mockRepo)
		newGSTIN := "33AAACC1206D1ZN"
		updated, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), nil, nil, nil, &newGSTIN, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected update partner profile to succeed, got %v", err)
		}
		if updated.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to remain false until VerifyGSTIN is called")
		}
	})

	t.Run("UpdatePartnerProfile GSTIN change invalidates existing approval and deactivates partner", func(t *testing.T) {
		oldGSTIN := "29ABCDE1234F1Z5"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			CompanyName:     "Acme Logistics",
			Email:           "partner@test.com",
			GSTIN:           &oldGSTIN,
			IsActive:        true,
			ApprovalStatus:  model.ApprovalStatusApproved,
			IsGSTINVerified: true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}
		svc := NewDeliveryService(mockRepo)
		newGSTIN := "33AAACC1206D1ZN"
		updated, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), nil, nil, nil, &newGSTIN, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected update partner profile to succeed, got %v", err)
		}
		if updated.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be false")
		}
		if updated.ApprovalStatus != model.ApprovalStatusPending {
			t.Fatalf("expected approval_status to be PENDING, got %s", updated.ApprovalStatus)
		}
		if updated.IsActive {
			t.Fatal("expected is_active to be false")
		}
		if saved.ApprovalStatus != model.ApprovalStatusPending || saved.IsActive || saved.IsGSTINVerified {
			t.Fatal("expected saved profile to reflect pending approval and inactive state")
		}

		// Delivery operations blocked while pending verification/approval
		orderConfirmedPayload := &dto.OrderConfirmedPayload{
			OrderID:         "ord-test-blocked",
			DeliveryPartner: partnerUUID.String(),
			ShippingAddress: &model.ShippingAddress{
				Street:        "123 Main St",
				City:          "San Francisco",
				State:         "CA",
				PostalCode:    "94105",
				Country:       "USA",
				ContactNumber: "+1-555-0199",
			},
		}
		_, err = svc.HandleOrderConfirmed(ctx, orderConfirmedPayload)
		if err == nil {
			t.Fatal("expected HandleOrderConfirmed to fail when partner is pending/inactive, got nil")
		}
		if !strings.Contains(err.Error(), "not approved or active") {
			t.Fatalf("expected error mentioning not approved or active, got %v", err)
		}

		// Partner cannot activate themselves while verification/approval is pending
		_, err = svc.ActivatePartner(ctx, partnerUUID.String())
		if err == nil {
			t.Fatal("expected ActivatePartner to fail when partner is not approved, got nil")
		}
		if !strings.Contains(err.Error(), "partner must be approved with verified GSTIN") {
			t.Fatalf("expected error mentioning partner must be approved with verified GSTIN, got %v", err)
		}
	})

	t.Run("UpdatePartnerProfile company name change invalidates existing approval and deactivates partner", func(t *testing.T) {
		validGSTIN := "33AAACC1206D1ZN"
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			CompanyName:     "Central Warehousing Corporation",
			Email:           "partner@test.com",
			GSTIN:           &validGSTIN,
			IsActive:        true,
			ApprovalStatus:  model.ApprovalStatusApproved,
			IsGSTINVerified: true,
		}
		var saved *model.DeliveryPartnerProfile
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
			upsertPartnerProfileFunc: func(ctx context.Context, p *model.DeliveryPartnerProfile) error {
				saved = p
				return nil
			},
		}
		svc := NewDeliveryService(mockRepo)
		newCompany := "BB.Stores"
		updated, err := svc.UpdatePartnerProfile(ctx, partnerUUID.String(), &newCompany, nil, nil, nil, partnerUUID.String())
		if err != nil {
			t.Fatalf("expected update partner profile to succeed, got %v", err)
		}
		if updated.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be false after company name change")
		}
		if updated.ApprovalStatus != model.ApprovalStatusPending {
			t.Fatalf("expected approval_status to be PENDING, got %s", updated.ApprovalStatus)
		}
		if updated.IsActive {
			t.Fatal("expected is_active to be false after company name change")
		}
		if saved.ApprovalStatus != model.ApprovalStatusPending || saved.IsActive || saved.IsGSTINVerified {
			t.Fatal("expected saved profile to reflect pending approval and unverified GSTIN")
		}
	})

	t.Run("VerifyGSTIN by GSTIN only looks up partner and succeeds", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:          uuid.New(),
			PartnerID:   partnerUUID,
			CompanyName: "Acme Logistics",
			Email:       "gstin-lookup@test.com",
			IsActive:    true,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileByGSTINFunc: func(ctx context.Context, gstin string) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}
		svc := NewDeliveryService(mockRepo, &mockGSTINClient{})
		res, err := svc.VerifyGSTIN(ctx, "", "33AAACC1206D1ZN", "")
		if err != nil {
			t.Fatalf("expected verify GSTIN by GSTIN lookup to succeed, got %v", err)
		}
		if !res.IsGSTINVerified {
			t.Fatal("expected is_gstin_verified to be true")
		}
	})

	t.Run("ApprovePartner fails if GSTIN is not provided or not verified", func(t *testing.T) {
		profile := &model.DeliveryPartnerProfile{
			ID:              uuid.New(),
			PartnerID:       partnerUUID,
			ApprovalStatus:  model.ApprovalStatusPending,
			IsActive:        true,
			IsGSTINVerified: false,
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			getPartnerProfileFunc: func(ctx context.Context, pid uuid.UUID) (*model.DeliveryPartnerProfile, error) {
				return profile, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.ApprovePartner(ctx, partnerUUID.String())
		if err == nil {
			t.Fatal("expected error when approving partner without verified GSTIN, got nil")
		}
	})
}

func TestPartnerOwnerOnly_ActivateDeactivate(t *testing.T) {
	partnerUUID := uuid.New()
	otherUUID := uuid.New()
	ctx := context.Background()

	t.Run("DeactivatePartner fails when reason is empty", func(t *testing.T) {
		svc := NewDeliveryService(&mockDeliveryRepoForServiceTest{})
		_, err := svc.DeactivatePartner(ctx, partnerUUID.String(), "   ", partnerUUID.String())
		if err == nil {
			t.Fatal("expected error for empty deactivation reason, got nil")
		}
	})

	t.Run("DeactivatePartner forbidden when caller is not owner", func(t *testing.T) {
		svc := NewDeliveryService(&mockDeliveryRepoForServiceTest{})
		_, err := svc.DeactivatePartner(ctx, partnerUUID.String(), "Reason", otherUUID.String())
		if err == nil {
			t.Fatal("expected forbidden error for non-owner deactivation, got nil")
		}
	})

	t.Run("ActivatePartner forbidden when caller is not owner", func(t *testing.T) {
		svc := NewDeliveryService(&mockDeliveryRepoForServiceTest{})
		_, err := svc.ActivatePartner(ctx, partnerUUID.String(), otherUUID.String())
		if err == nil {
			t.Fatal("expected forbidden error for non-owner activation, got nil")
		}
	})

	t.Run("ListAppealedPartners returns appealed partners", func(t *testing.T) {
		appealed := []*model.DeliveryPartnerProfile{
			{
				ID:             uuid.New(),
				PartnerID:      partnerUUID,
				ApprovalStatus: model.ApprovalStatusAppealed,
			},
		}
		mockRepo := &mockDeliveryRepoForServiceTest{
			listAppealedPartnersFunc: func(ctx context.Context, limit, offset int) ([]*model.DeliveryPartnerProfile, int, error) {
				return appealed, 1, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		list, total, err := svc.ListAppealedPartners(ctx, 10, 0)
		if err != nil {
			t.Fatalf("expected success, got %v", err)
		}
		if total != 1 || len(list) != 1 || list[0].ApprovalStatus != model.ApprovalStatusAppealed {
			t.Fatalf("unexpected list result: %+v", list)
		}
	})
}

func TestMatchCompanyName_ExactNormalizedMatching(t *testing.T) {
	tests := []struct {
		name           string
		partnerCompany string
		legalName      string
		tradeName      string
		want           bool
	}{
		{
			name:           "exact match with legal name ignoring corporate suffixes and case",
			partnerCompany: "Acme Logistics",
			legalName:      "ACME LOGISTICS PRIVATE LIMITED",
			want:           true,
		},
		{
			name:           "exact match with trade name",
			partnerCompany: "Flash Express",
			legalName:      "FLASH HOLDINGS INC",
			tradeName:      "FLASH EXPRESS CO",
			want:           true,
		},
		{
			name:           "reject substring of legal name",
			partnerCompany: "Logistics",
			legalName:      "ACME LOGISTICS PRIVATE LIMITED",
			want:           false,
		},
		{
			name:           "reject superstring of legal name",
			partnerCompany: "Acme Logistics Worldwide Express",
			legalName:      "ACME LOGISTICS PRIVATE LIMITED",
			want:           false,
		},
		{
			name:           "reject subset words",
			partnerCompany: "Acme",
			legalName:      "ACME PHARMA AND CHEMICALS",
			want:           false,
		},
		{
			name:           "reject empty partner company",
			partnerCompany: "   ",
			legalName:      "ACME LOGISTICS PRIVATE LIMITED",
			want:           false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := matchCompanyName(tt.partnerCompany, tt.legalName, tt.tradeName)
			if got != tt.want {
				t.Errorf("matchCompanyName(%q, %q, %q) = %v, want %v", tt.partnerCompany, tt.legalName, tt.tradeName, got, tt.want)
			}
		})
	}
}

func TestDeliveryService_CancelDelivery(t *testing.T) {
	ctx := context.Background()

	t.Run("successful cancellation from CREATED state", func(t *testing.T) {
		deliveryID := uuid.New().String()
		orderID := uuid.New().String()
		d := &model.Delivery{
			ID:      deliveryID,
			OrderID: orderID,
			Status:  model.StatusCreated,
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getByIDFunc: func(ctx context.Context, id string) (*model.Delivery, error) {
				return d, nil
			},
			updateStatusWithDetailsFunc: func(ctx context.Context, id string, status model.DeliveryStatus, est, act *time.Time, loc, notes string) (*model.Delivery, error) {
				d.Status = status
				return d, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.CancelDelivery(ctx, deliveryID, "", "Customer cancelled")
		if err != nil {
			t.Fatalf("expected cancel to succeed, got %v", err)
		}
		if res.Status != model.StatusCancelled {
			t.Fatalf("expected status CANCELLED, got %s", res.Status)
		}
	})

	t.Run("rejection when parcel already transitioned to PICKED_UP", func(t *testing.T) {
		deliveryID := uuid.New().String()
		d := &model.Delivery{
			ID:      deliveryID,
			OrderID: uuid.New().String(),
			Status:  model.StatusPickedUp,
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getByIDFunc: func(ctx context.Context, id string) (*model.Delivery, error) {
				return d, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.CancelDelivery(ctx, deliveryID, "", "Too late")
		if err == nil {
			t.Fatalf("expected cancellation of PICKED_UP delivery to be rejected")
		}
	})

	t.Run("rejection when parcel in terminal DELIVERED state", func(t *testing.T) {
		deliveryID := uuid.New().String()
		d := &model.Delivery{
			ID:      deliveryID,
			OrderID: uuid.New().String(),
			Status:  model.StatusDelivered,
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getByIDFunc: func(ctx context.Context, id string) (*model.Delivery, error) {
				return d, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		_, err := svc.CancelDelivery(ctx, deliveryID, "", "Package arrived")
		if err == nil {
			t.Fatalf("expected cancellation of DELIVERED delivery to be rejected")
		}
	})

	t.Run("idempotent cancellation if already CANCELLED", func(t *testing.T) {
		deliveryID := uuid.New().String()
		d := &model.Delivery{
			ID:      deliveryID,
			OrderID: uuid.New().String(),
			Status:  model.StatusCancelled,
		}

		mockRepo := &mockDeliveryRepoForServiceTest{
			getByIDFunc: func(ctx context.Context, id string) (*model.Delivery, error) {
				return d, nil
			},
		}

		svc := NewDeliveryService(mockRepo)
		res, err := svc.CancelDelivery(ctx, deliveryID, "", "Duplicate cancel")
		if err != nil || res.Status != model.StatusCancelled {
			t.Fatalf("expected idempotent success, got %v", err)
		}
	})
}
