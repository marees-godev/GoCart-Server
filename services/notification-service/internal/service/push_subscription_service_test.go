package service_test

import (
	"bytes"
	"context"
	"crypto/ecdh"
	"crypto/rand"
	"encoding/base64"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/config"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/push"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type memPushSubRepo struct {
	mu           sync.RWMutex
	byID         map[string]*model.PushSubscription
	byEndpoint   map[string]string
	goneCalls    int
	failureCalls int
	usedCalls    int
}

func newMemPushSubRepo() *memPushSubRepo {
	return &memPushSubRepo{
		byID:       make(map[string]*model.PushSubscription),
		byEndpoint: make(map[string]string),
	}
}

func clonePushSub(s *model.PushSubscription) *model.PushSubscription {
	if s == nil {
		return nil
	}
	cp := *s
	if s.LastError != nil {
		v := *s.LastError
		cp.LastError = &v
	}
	if s.LastUsedAt != nil {
		v := *s.LastUsedAt
		cp.LastUsedAt = &v
	}
	if s.DeactivatedAt != nil {
		v := *s.DeactivatedAt
		cp.DeactivatedAt = &v
	}
	return &cp
}

func (r *memPushSubRepo) UpsertSubscription(ctx context.Context, sub *model.PushSubscription) (*model.PushSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	now := time.Now().UTC()
	if existingID, ok := r.byEndpoint[sub.Endpoint]; ok {
		existing := r.byID[existingID]
		existing.UserID = sub.UserID
		existing.P256DH = sub.P256DH
		existing.Auth = sub.Auth
		if sub.UserAgent != "" {
			existing.UserAgent = sub.UserAgent
		}
		if sub.DeviceName != "" {
			existing.DeviceName = sub.DeviceName
		}
		existing.IsActive = true
		existing.FailureCount = 0
		existing.LastError = nil
		existing.DeactivatedAt = nil
		existing.UpdatedAt = now
		return clonePushSub(existing), nil
	}

	cp := clonePushSub(sub)
	if cp.ID == "" {
		cp.ID = uuid.NewString()
	}
	cp.IsActive = true
	cp.FailureCount = 0
	cp.CreatedAt = now
	cp.UpdatedAt = now
	r.byID[cp.ID] = cp
	r.byEndpoint[cp.Endpoint] = cp.ID
	return clonePushSub(cp), nil
}

func (r *memPushSubRepo) GetSubscriptionByID(ctx context.Context, id string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return clonePushSub(r.byID[id]), nil
}

func (r *memPushSubRepo) GetSubscriptionByEndpoint(ctx context.Context, endpoint string) (*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	id, ok := r.byEndpoint[endpoint]
	if !ok {
		return nil, nil
	}
	return clonePushSub(r.byID[id]), nil
}

func (r *memPushSubRepo) ListSubscriptionsByUserID(ctx context.Context, userID string, activeOnly bool) ([]*model.PushSubscription, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var result []*model.PushSubscription
	for _, s := range r.byID {
		if s.UserID != userID {
			continue
		}
		if activeOnly && !s.IsActive {
			continue
		}
		result = append(result, clonePushSub(s))
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].CreatedAt.Before(result[j].CreatedAt)
	})
	return result, nil
}

func (r *memPushSubRepo) UpdateSubscription(ctx context.Context, patch *model.PushSubscription) (*model.PushSubscription, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	existing, ok := r.byID[patch.ID]
	if !ok || (patch.UserID != "" && existing.UserID != patch.UserID) {
		return nil, nil
	}
	if patch.Endpoint != "" && patch.Endpoint != existing.Endpoint {
		if _, collision := r.byEndpoint[patch.Endpoint]; collision {
			return nil, appErrors.Conflict("endpoint already registered")
		}
		delete(r.byEndpoint, existing.Endpoint)
		existing.Endpoint = patch.Endpoint
		r.byEndpoint[patch.Endpoint] = existing.ID
	}
	if patch.P256DH != "" {
		existing.P256DH = patch.P256DH
	}
	if patch.Auth != "" {
		existing.Auth = patch.Auth
	}
	if patch.UserAgent != "" {
		existing.UserAgent = patch.UserAgent
	}
	if patch.DeviceName != "" {
		existing.DeviceName = patch.DeviceName
	}
	now := time.Now().UTC()
	existing.IsActive = patch.IsActive
	if patch.IsActive {
		existing.DeactivatedAt = nil
		existing.FailureCount = 0
		existing.LastError = nil
	} else if existing.DeactivatedAt == nil {
		existing.DeactivatedAt = &now
	}
	existing.UpdatedAt = now
	return clonePushSub(existing), nil
}

func (r *memPushSubRepo) DeactivateSubscription(ctx context.Context, id, userID, endpoint string) (bool, error) {
	r.mu.Lock()
	defer r.mu.Unlock()

	var target *model.PushSubscription
	if id != "" {
		target = r.byID[id]
	} else if endpoint != "" {
		if subID, ok := r.byEndpoint[endpoint]; ok {
			target = r.byID[subID]
		}
	}
	if target == nil || (userID != "" && target.UserID != userID) {
		return false, nil
	}
	now := time.Now().UTC()
	target.IsActive = false
	if target.DeactivatedAt == nil {
		target.DeactivatedAt = &now
	}
	target.UpdatedAt = now
	return true, nil
}

func (r *memPushSubRepo) MarkSubscriptionGone(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.goneCalls++
	if s, ok := r.byID[id]; ok {
		now := time.Now().UTC()
		s.IsActive = false
		s.FailureCount++
		s.LastError = &errMsg
		if s.DeactivatedAt == nil {
			s.DeactivatedAt = &now
		}
		s.UpdatedAt = now
	}
	return nil
}

func (r *memPushSubRepo) RecordSubscriptionFailure(ctx context.Context, id string, errMsg string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.failureCalls++
	if s, ok := r.byID[id]; ok {
		s.FailureCount++
		s.LastError = &errMsg
		s.UpdatedAt = time.Now().UTC()
	}
	return nil
}

func (r *memPushSubRepo) MarkSubscriptionUsed(ctx context.Context, id string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.usedCalls++
	if s, ok := r.byID[id]; ok {
		now := time.Now().UTC()
		s.FailureCount = 0
		s.LastError = nil
		s.LastUsedAt = &now
		s.UpdatedAt = now
	}
	return nil
}

func generateTestSubKeys(t *testing.T) (*ecdh.PrivateKey, string, string) {
	t.Helper()
	priv, err := ecdh.P256().GenerateKey(rand.Reader)
	if err != nil {
		t.Fatalf("failed generating ECDH key: %v", err)
	}
	authSecret := make([]byte, 16)
	if _, err := rand.Read(authSecret); err != nil {
		t.Fatalf("failed generating auth secret: %v", err)
	}
	return priv,
		base64.RawURLEncoding.EncodeToString(priv.PublicKey().Bytes()),
		base64.RawURLEncoding.EncodeToString(authSecret)
}

func TestPushSubscriptionService_RegisterMultiDeviceAndValidation(t *testing.T) {
	ctx := context.Background()
	pubVAPID, privVAPID, err := push.GenerateVAPIDKeys()
	if err != nil {
		t.Fatalf("GenerateVAPIDKeys failed: %v", err)
	}

	var logBuf bytes.Buffer
	log := slog.New(slog.NewJSONHandler(&logBuf, nil))
	repo := newMemPushSubRepo()
	svc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{
		PublicKey:  pubVAPID,
		PrivateKey: privVAPID,
		Subject:    "mailto:push@gocart.local",
	}, log)

	userID := uuid.NewString()
	_, p256_1, auth_1 := generateTestSubKeys(t)
	_, p256_2, auth_2 := generateTestSubKeys(t)

	// Register first device (Chrome)
	sub1, err := svc.Register(ctx, dto.RegisterPushSubscriptionInput{
		UserID:     userID,
		Endpoint:   "https://fcm.googleapis.com/fcm/send/device-1",
		P256DH:     p256_1,
		Auth:       auth_1,
		UserAgent:  "Chrome/132.0",
		DeviceName: "Work Laptop Chrome",
	})
	if err != nil {
		t.Fatalf("Register device 1 failed: %v", err)
	}
	if !sub1.IsActive || sub1.UserID != userID {
		t.Fatalf("unexpected sub1: %+v", sub1)
	}

	// Register second device (Firefox) for same user
	sub2, err := svc.Register(ctx, dto.RegisterPushSubscriptionInput{
		UserID:     userID,
		Endpoint:   "https://updates.push.services.mozilla.com/wpush/v2/device-2",
		P256DH:     p256_2,
		Auth:       auth_2,
		UserAgent:  "Firefox/134.0",
		DeviceName: "Personal Firefox",
	})
	if err != nil {
		t.Fatalf("Register device 2 failed: %v", err)
	}
	if sub2.ID == sub1.ID {
		t.Fatalf("expected distinct subscription IDs for multiple devices")
	}

	subs, err := svc.GetByUserID(ctx, userID, true)
	if err != nil {
		t.Fatalf("GetByUserID failed: %v", err)
	}
	if len(subs) != 2 {
		t.Fatalf("expected 2 active subscriptions, got %d", len(subs))
	}

	// Re-registering existing endpoint upserts rather than duplicating
	_, newP256, newAuth := generateTestSubKeys(t)
	reUpserted, err := svc.Register(ctx, dto.RegisterPushSubscriptionInput{
		UserID:     userID,
		Endpoint:   sub1.Endpoint,
		P256DH:     newP256,
		Auth:       newAuth,
		DeviceName: "Work Laptop Chrome Updated",
	})
	if err != nil {
		t.Fatalf("re-upsert failed: %v", err)
	}
	if reUpserted.ID != sub1.ID || reUpserted.DeviceName != "Work Laptop Chrome Updated" {
		t.Fatalf("unexpected re-upsert result: %+v", reUpserted)
	}

	// Ensure push subscription secrets and VAPID private key never appear in logs
	logs := logBuf.String()
	for _, secret := range []string{p256_1, auth_1, p256_2, auth_2, newP256, newAuth, privVAPID} {
		if strings.Contains(logs, secret) {
			t.Fatalf("security violation: secret leaked in logs")
		}
	}

	// Validation errors
	badInputs := []struct {
		name string
		in   dto.RegisterPushSubscriptionInput
	}{
		{"invalid user_id", dto.RegisterPushSubscriptionInput{UserID: "not-a-uuid", Endpoint: "https://push.example.com/1", P256DH: p256_1, Auth: auth_1}},
		{"invalid scheme", dto.RegisterPushSubscriptionInput{UserID: userID, Endpoint: "ftp://push.example.com/1", P256DH: p256_1, Auth: auth_1}},
		{"bad p256dh", dto.RegisterPushSubscriptionInput{UserID: userID, Endpoint: "https://push.example.com/1", P256DH: "short", Auth: auth_1}},
		{"bad auth", dto.RegisterPushSubscriptionInput{UserID: userID, Endpoint: "https://push.example.com/1", P256DH: p256_1, Auth: "short"}},
	}
	for _, tc := range badInputs {
		if _, err := svc.Register(ctx, tc.in); appErrors.AsAppError(err).Code != appErrors.CodeBadRequest {
			t.Errorf("%s: expected BAD_REQUEST, got %v", tc.name, err)
		}
	}
}

func TestPushSubscriptionService_UpdateDeactivateAndVAPID(t *testing.T) {
	ctx := context.Background()
	pubVAPID, privVAPID, _ := push.GenerateVAPIDKeys()
	repo := newMemPushSubRepo()
	svc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{
		PublicKey:  pubVAPID,
		PrivateKey: privVAPID,
	}, slog.New(slog.DiscardHandler))

	ownerID := uuid.NewString()
	otherUserID := uuid.NewString()
	_, p256Key, authKey := generateTestSubKeys(t)

	sub, err := svc.Register(ctx, dto.RegisterPushSubscriptionInput{
		UserID:     ownerID,
		Endpoint:   "https://push.example.com/u1",
		P256DH:     p256Key,
		Auth:       authKey,
		DeviceName: "Pixel 9",
	})
	if err != nil {
		t.Fatalf("Register failed: %v", err)
	}

	// Owner updates device name and disables temporarily
	isActiveFalse := false
	updated, err := svc.Update(ctx, dto.UpdatePushSubscriptionInput{
		ID:         sub.ID,
		UserID:     ownerID,
		DeviceName: "Pixel 9 Pro",
		IsActive:   &isActiveFalse,
	})
	if err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if updated.DeviceName != "Pixel 9 Pro" || updated.IsActive {
		t.Fatalf("unexpected updated sub: %+v", updated)
	}

	// ActiveOnly filters out disabled subscription
	activeSubs, err := svc.GetByUserID(ctx, ownerID, true)
	if err != nil || len(activeSubs) != 0 {
		t.Fatalf("expected 0 active subs, got %d (err=%v)", len(activeSubs), err)
	}
	allSubs, err := svc.GetByUserID(ctx, ownerID, false)
	if err != nil || len(allSubs) != 1 {
		t.Fatalf("expected 1 total sub, got %d (err=%v)", len(allSubs), err)
	}

	// Update by endpoint reactivates
	isActiveTrue := true
	reactivated, err := svc.Update(ctx, dto.UpdatePushSubscriptionInput{
		UserID:   ownerID,
		Endpoint: sub.Endpoint,
		IsActive: &isActiveTrue,
	})
	if err != nil || !reactivated.IsActive {
		t.Fatalf("expected reactivation by endpoint, got %+v (err=%v)", reactivated, err)
	}

	// Cross-user update & deactivate forbidden
	if _, err := svc.Update(ctx, dto.UpdatePushSubscriptionInput{ID: sub.ID, UserID: otherUserID, DeviceName: "Hack"}); appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN on cross-user update, got %v", err)
	}
	if _, err := svc.Deactivate(ctx, dto.DeactivatePushSubscriptionInput{ID: sub.ID, UserID: otherUserID}); appErrors.AsAppError(err).Code != appErrors.CodeForbidden {
		t.Errorf("expected FORBIDDEN on cross-user deactivate, got %v", err)
	}

	// Non-existent lookup returns NotFound
	if _, err := svc.Deactivate(ctx, dto.DeactivatePushSubscriptionInput{ID: uuid.NewString(), UserID: ownerID}); appErrors.AsAppError(err).Code != appErrors.CodeNotFound {
		t.Errorf("expected NOT_FOUND on missing deactivate, got %v", err)
	}

	// Owner deactivates by endpoint
	ok, err := svc.Deactivate(ctx, dto.DeactivatePushSubscriptionInput{UserID: ownerID, Endpoint: sub.Endpoint})
	if err != nil || !ok {
		t.Fatalf("expected Deactivate success, got ok=%v err=%v", ok, err)
	}

	// VAPID public key retrieval (configured vs unconfigured)
	gotPub, gotSubj, err := svc.GetVAPIDPublicKey(ctx)
	if err != nil || gotPub != pubVAPID || gotSubj != "mailto:notifications@gocart.local" {
		t.Fatalf("unexpected VAPID response: pub=%s subj=%s err=%v", gotPub, gotSubj, err)
	}

	unconfiguredSvc := service.NewPushSubscriptionService(repo, config.VAPIDConfig{}, slog.New(slog.DiscardHandler))
	if _, _, err := unconfiguredSvc.GetVAPIDPublicKey(ctx); appErrors.AsAppError(err).Code != appErrors.CodeServiceUnavailable {
		t.Errorf("expected SERVICE_UNAVAILABLE for unconfigured VAPID, got %v", err)
	}
}

