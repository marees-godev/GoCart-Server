package model_test

import (
	"fmt"
	"testing"

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

	// Valid transitions expected to succeed
	validTransitions := map[string]bool{
		"PENDING->APPROVED":   true,
		"PENDING->REJECTED":   true,
		"APPROVED->SUSPENDED": true,
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

	if validCount != 3 {
		t.Errorf("expected exactly 3 valid transitions, got %d", validCount)
	}
	if invalidCount != 13 {
		t.Errorf("expected exactly 13 invalid transitions, got %d", invalidCount)
	}
}

func TestInvalidStateTransitionErrorFormatting(t *testing.T) {
	validator := model.NewStateTransitionValidator()

	// 1. PENDING -> SUSPENDED
	err := validator.Validate(model.MerchantStatusPending, model.MerchantStatusSuspended)
	if err == nil {
		t.Fatal("expected error for PENDING -> SUSPENDED")
	}
	expectedMsg := "Cannot transition merchant from 'PENDING' to 'SUSPENDED'. Allowed targets: ['APPROVED', 'REJECTED']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 2. APPROVED -> PENDING
	err = validator.Validate(model.MerchantStatusApproved, model.MerchantStatusPending)
	if err == nil {
		t.Fatal("expected error for APPROVED -> PENDING")
	}
	expectedMsg = "Cannot transition merchant from 'APPROVED' to 'PENDING'. Allowed targets: ['SUSPENDED']."
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

	// 4. SUSPENDED -> PENDING
	err = validator.Validate(model.MerchantStatusSuspended, model.MerchantStatusPending)
	if err == nil {
		t.Fatal("expected error for SUSPENDED -> PENDING")
	}
	expectedMsg = "Cannot transition merchant from 'SUSPENDED' to 'PENDING'. Allowed targets: []."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}

	// 5. Self transition APPROVED -> APPROVED
	err = validator.Validate(model.MerchantStatusApproved, model.MerchantStatusApproved)
	if err == nil {
		t.Fatal("expected error for APPROVED -> APPROVED")
	}
	expectedMsg = "Cannot transition merchant from 'APPROVED' to 'APPROVED'. Allowed targets: ['SUSPENDED']."
	if err.Error() != expectedMsg {
		t.Errorf("expected error message:\n%q\ngot:\n%q", expectedMsg, err.Error())
	}
}

func TestIsValidStatus(t *testing.T) {
	valid := []string{"PENDING", "APPROVED", "REJECTED", "SUSPENDED", "pending", "approved", "rejected", "suspended"}
	for _, s := range valid {
		if !model.IsValidStatus(s) {
			t.Errorf("expected %q to be valid status", s)
		}
	}

	invalid := []string{"", "ACTIVE", "DELETED", "UNKNOWN", "123"}
	for _, s := range invalid {
		if model.IsValidStatus(s) {
			t.Errorf("expected %q to be invalid status", s)
		}
	}
}
