package model_test

import (
	"fmt"
	"net/http"
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/merchant-service/internal/model"
)

func TestStateTransitionMatrix(t *testing.T) {
	validator := model.NewStateTransitionValidator()

	allStatuses := []model.MerchantStatus{
		model.MerchantStatusPending,
		model.MerchantStatusApproved,
		model.MerchantStatusRejected,
		model.MerchantStatusSuspended,
	}

	validTransitions := map[string]bool{
		"PENDING->APPROVED":   true,
		"PENDING->REJECTED":   true,
		"APPROVED->SUSPENDED": true,
		"SUSPENDED->PENDING":  true,
	}

	validCount := 0
	invalidCount := 0

	for _, from := range allStatuses {
		for _, to := range allStatuses {
			key := fmt.Sprintf("%s->%s", from, to)
			err := validator.Validate(from, to)

			if validTransitions[key] {
				validCount++
				if err != nil {
					t.Errorf("expected transition %s to be VALID, but got error: %v", key, err)
				}
			} else {
				invalidCount++
				if err == nil {
					t.Errorf("expected transition %s to be INVALID, but it passed", key)
				} else {
					transErr, ok := err.(*model.InvalidStateTransitionError)
					if !ok {
						t.Errorf("expected *model.InvalidStateTransitionError, got %T", err)
					} else {
						if transErr.FromStatus != from {
							t.Errorf("expected FromStatus %s, got %s", from, transErr.FromStatus)
						}
						if transErr.ToStatus != to {
							t.Errorf("expected ToStatus %s, got %s", to, transErr.ToStatus)
						}
					}
				}
			}
		}
	}

	if validCount != 4 {
		t.Errorf("expected exactly 4 valid transitions, got %d", validCount)
	}
	if invalidCount != 12 {
		t.Errorf("expected exactly 12 invalid transitions, got %d", invalidCount)
	}
}

func TestInvalidStateTransitionErrorFormatting(t *testing.T) {
	validator := model.NewStateTransitionValidator()

	// 1. PENDING -> SUSPENDED
	err := validator.Validate(model.MerchantStatusPending, model.MerchantStatusSuspended)
	if err == nil {
		t.Fatal("expected error for PENDING -> SUSPENDED")
	}
	expectedMsg := "Cannot transition merchant from 'PENDING' to 'SUSPENDED'. Allowed targets: ['APPROVED', 'REJECTED', 'ACTIVE']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 2. APPROVED -> PENDING
	err = validator.Validate(model.MerchantStatusApproved, model.MerchantStatusPending)
	if err == nil {
		t.Fatal("expected error for APPROVED -> PENDING")
	}
	expectedMsg = "Cannot transition merchant from 'APPROVED' to 'PENDING'. Allowed targets: ['SUSPENDED', 'ACTIVE']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 3. REJECTED -> APPROVED (terminal state)
	err = validator.Validate(model.MerchantStatusRejected, model.MerchantStatusApproved)
	if err == nil {
		t.Fatal("expected error for REJECTED -> APPROVED")
	}
	expectedMsg = "Cannot transition merchant from 'REJECTED' to 'APPROVED'. Allowed targets: []."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 4. SUSPENDED -> REJECTED
	err = validator.Validate(model.MerchantStatusSuspended, model.MerchantStatusRejected)
	if err == nil {
		t.Fatal("expected error for SUSPENDED -> REJECTED")
	}
	expectedMsg = "Cannot transition merchant from 'SUSPENDED' to 'REJECTED'. Allowed targets: ['ACTIVE', 'PENDING', 'TERMINATED']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 5. Self transition APPROVED -> APPROVED
	err = validator.Validate(model.MerchantStatusApproved, model.MerchantStatusApproved)
	if err == nil {
		t.Fatal("expected error for APPROVED -> APPROVED")
	}
	expectedMsg = "Cannot transition merchant from 'APPROVED' to 'APPROVED'. Allowed targets: ['SUSPENDED', 'ACTIVE']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []string{
		"PENDING", "APPROVED", "REJECTED", "SUSPENDED",
		"ACTIVE", "INACTIVE", "PENDING_REVIEW", "TERMINATED",
		"pending", "approved", "rejected", "suspended",
		"active", "inactive", "pending_review", "terminated",
	}
	for _, s := range valid {
		if !model.IsValidStatus(s) {
			t.Errorf("expected %q to be valid status", s)
		}
	}

	invalid := []string{"", "INVALID_STATUS", "DELETED", "UNKNOWN", "123"}
	for _, s := range invalid {
		if model.IsValidStatus(s) {
			t.Errorf("expected %q to be invalid status", s)
		}
	}
}

func TestValidateLifecycleTransition_Activate(t *testing.T) {
	tests := []struct {
		name          string
		currentStatus model.MerchantStatus
		expectTarget  model.MerchantStatus
		expectErrCode string
		expectHTTP    int
	}{
		{
			name:          "Activate from PENDING -> ACTIVE (Success)",
			currentStatus: model.MerchantStatusPending,
			expectTarget:  model.MerchantStatusActive,
		},
		{
			name:          "Activate from PENDING_REVIEW -> ACTIVE (Success)",
			currentStatus: model.MerchantStatusPendingReview,
			expectTarget:  model.MerchantStatusActive,
		},
		{
			name:          "Activate from INACTIVE -> ACTIVE (Success)",
			currentStatus: model.MerchantStatusInactive,
			expectTarget:  model.MerchantStatusActive,
		},
		{
			name:          "Activate from APPROVED -> ACTIVE (Success)",
			currentStatus: model.MerchantStatusApproved,
			expectTarget:  model.MerchantStatusActive,
		},
		{
			name:          "Activate from ACTIVE -> Conflict (Already ACTIVE)",
			currentStatus: model.MerchantStatusActive,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Activate from TERMINATED -> Conflict",
			currentStatus: model.MerchantStatusTerminated,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Activate from SUSPENDED -> Conflict (Must reactivate)",
			currentStatus: model.MerchantStatusSuspended,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := model.ValidateLifecycleTransition(model.LifecycleActionActivate, tt.currentStatus)
			if tt.expectErrCode != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != tt.expectErrCode {
					t.Fatalf("expected error code %s, got %v", tt.expectErrCode, err)
				}
				if appErr.HTTPStatus != tt.expectHTTP {
					t.Fatalf("expected HTTP status %d, got %d", tt.expectHTTP, appErr.HTTPStatus)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if target != tt.expectTarget {
					t.Fatalf("expected target %s, got %s", tt.expectTarget, target)
				}
			}
		})
	}
}

func TestValidateLifecycleTransition_Suspend(t *testing.T) {
	tests := []struct {
		name          string
		currentStatus model.MerchantStatus
		expectTarget  model.MerchantStatus
		expectErrCode string
		expectHTTP    int
	}{
		{
			name:          "Suspend from ACTIVE -> SUSPENDED (Success)",
			currentStatus: model.MerchantStatusActive,
			expectTarget:  model.MerchantStatusSuspended,
		},
		{
			name:          "Suspend from APPROVED -> SUSPENDED (Success)",
			currentStatus: model.MerchantStatusApproved,
			expectTarget:  model.MerchantStatusSuspended,
		},
		{
			name:          "Suspend from SUSPENDED -> Conflict (Already SUSPENDED)",
			currentStatus: model.MerchantStatusSuspended,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Suspend from TERMINATED -> Conflict",
			currentStatus: model.MerchantStatusTerminated,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Suspend from PENDING -> Conflict (Not active)",
			currentStatus: model.MerchantStatusPending,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Suspend from INACTIVE -> Conflict (Not active)",
			currentStatus: model.MerchantStatusInactive,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := model.ValidateLifecycleTransition(model.LifecycleActionSuspend, tt.currentStatus)
			if tt.expectErrCode != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != tt.expectErrCode {
					t.Fatalf("expected error code %s, got %v", tt.expectErrCode, err)
				}
				if appErr.HTTPStatus != tt.expectHTTP {
					t.Fatalf("expected HTTP status %d, got %d", tt.expectHTTP, appErr.HTTPStatus)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if target != tt.expectTarget {
					t.Fatalf("expected target %s, got %s", tt.expectTarget, target)
				}
			}
		})
	}
}

func TestValidateLifecycleTransition_Reactivate(t *testing.T) {
	tests := []struct {
		name          string
		currentStatus model.MerchantStatus
		expectTarget  model.MerchantStatus
		expectErrCode string
		expectHTTP    int
	}{
		{
			name:          "Reactivate from SUSPENDED -> ACTIVE (Success)",
			currentStatus: model.MerchantStatusSuspended,
			expectTarget:  model.MerchantStatusActive,
		},
		{
			name:          "Reactivate from ACTIVE -> Conflict (Already ACTIVE)",
			currentStatus: model.MerchantStatusActive,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Reactivate from PENDING_REVIEW -> Conflict (Not SUSPENDED)",
			currentStatus: model.MerchantStatusPendingReview,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Reactivate from PENDING -> Conflict (Not SUSPENDED)",
			currentStatus: model.MerchantStatusPending,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
		{
			name:          "Reactivate from TERMINATED -> Conflict",
			currentStatus: model.MerchantStatusTerminated,
			expectErrCode: appErrors.CodeConflict,
			expectHTTP:    http.StatusConflict,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			target, err := model.ValidateLifecycleTransition(model.LifecycleActionReactivate, tt.currentStatus)
			if tt.expectErrCode != "" {
				if err == nil {
					t.Fatalf("expected error, got nil")
				}
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != tt.expectErrCode {
					t.Fatalf("expected error code %s, got %v", tt.expectErrCode, err)
				}
				if appErr.HTTPStatus != tt.expectHTTP {
					t.Fatalf("expected HTTP status %d, got %d", tt.expectHTTP, appErr.HTTPStatus)
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if target != tt.expectTarget {
					t.Fatalf("expected target %s, got %s", tt.expectTarget, target)
				}
			}
		})
	}
}

func TestValidateLifecycleTransition_InvalidAction(t *testing.T) {
	_, err := model.ValidateLifecycleTransition(model.LifecycleAction("UNKNOWN"), model.MerchantStatusActive)
	if err == nil {
		t.Fatalf("expected error for unknown action")
	}
	appErr := appErrors.AsAppError(err)
	if appErr == nil || appErr.Code != appErrors.CodeBadRequest {
		t.Fatalf("expected BAD_REQUEST, got %v", err)
	}
}
