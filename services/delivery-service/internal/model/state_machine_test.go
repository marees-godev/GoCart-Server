package model_test

import (
	"errors"
	"net/http"
	"testing"

	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

func TestDeliveryStateMachine_ValidTransitions(t *testing.T) {
	validCases := []struct {
		name    string
		current model.DeliveryStatus
		target  model.DeliveryStatus
	}{
		{"CREATED to PICKED_UP", model.StatusCreated, model.StatusPickedUp},
		{"CREATED to CANCELLED", model.StatusCreated, model.StatusCancelled},
		{"CREATED to FAILED", model.StatusCreated, model.StatusFailed},
		{"PICKED_UP to IN_TRANSIT", model.StatusPickedUp, model.StatusInTransit},
		{"PICKED_UP to FAILED", model.StatusPickedUp, model.StatusFailed},
		{"IN_TRANSIT to OUT_FOR_DELIVERY", model.StatusInTransit, model.StatusOutForDelivery},
		{"IN_TRANSIT to FAILED", model.StatusInTransit, model.StatusFailed},
		{"OUT_FOR_DELIVERY to DELIVERED", model.StatusOutForDelivery, model.StatusDelivered},
		{"OUT_FOR_DELIVERY to FAILED", model.StatusOutForDelivery, model.StatusFailed},
	}

	for _, tc := range validCases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.TransitionStatus(tc.current, tc.target)
			if err != nil {
				t.Fatalf("expected valid transition from %s to %s, got error: %v", tc.current, tc.target, err)
			}
		})
	}
}

func TestDeliveryStateMachine_FullLifecycleHappyPath(t *testing.T) {
	delivery := &model.Delivery{
		ID:     "test-del-1",
		Status: model.StatusCreated,
	}

	steps := []model.DeliveryStatus{
		model.StatusPickedUp,
		model.StatusInTransit,
		model.StatusOutForDelivery,
		model.StatusDelivered,
	}

	for _, nextStatus := range steps {
		if err := delivery.TransitionStatus(nextStatus); err != nil {
			t.Fatalf("expected transition to %s to succeed, got %v", nextStatus, err)
		}
		if delivery.Status != nextStatus {
			t.Fatalf("expected delivery status to be %s, got %s", nextStatus, delivery.Status)
		}
	}
}

func TestDeliveryStateMachine_InvalidTransitions(t *testing.T) {
	invalidCases := []struct {
		name    string
		current model.DeliveryStatus
		target  model.DeliveryStatus
	}{
		// Skipping states
		{"CREATED to IN_TRANSIT", model.StatusCreated, model.StatusInTransit},
		{"CREATED to OUT_FOR_DELIVERY", model.StatusCreated, model.StatusOutForDelivery},
		{"CREATED to DELIVERED", model.StatusCreated, model.StatusDelivered},
		{"PICKED_UP to OUT_FOR_DELIVERY", model.StatusPickedUp, model.StatusOutForDelivery},
		{"PICKED_UP to DELIVERED", model.StatusPickedUp, model.StatusDelivered},
		{"IN_TRANSIT to DELIVERED", model.StatusInTransit, model.StatusDelivered},
		{"PICKED_UP to CANCELLED", model.StatusPickedUp, model.StatusCancelled},

		// Backward transitions
		{"PICKED_UP to CREATED", model.StatusPickedUp, model.StatusCreated},
		{"IN_TRANSIT to PICKED_UP", model.StatusInTransit, model.StatusPickedUp},
		{"OUT_FOR_DELIVERY to IN_TRANSIT", model.StatusOutForDelivery, model.StatusInTransit},

		// Out of terminal states
		{"DELIVERED to IN_TRANSIT", model.StatusDelivered, model.StatusInTransit},
		{"DELIVERED to CREATED", model.StatusDelivered, model.StatusCreated},
		{"DELIVERED to FAILED", model.StatusDelivered, model.StatusFailed},
		{"DELIVERED to CANCELLED", model.StatusDelivered, model.StatusCancelled},

		{"FAILED to CREATED", model.StatusFailed, model.StatusCreated},
		{"FAILED to PICKED_UP", model.StatusFailed, model.StatusPickedUp},
		{"FAILED to IN_TRANSIT", model.StatusFailed, model.StatusInTransit},
		{"FAILED to DELIVERED", model.StatusFailed, model.StatusDelivered},

		{"CANCELLED to CREATED", model.StatusCancelled, model.StatusCreated},
		{"CANCELLED to PICKED_UP", model.StatusCancelled, model.StatusPickedUp},
		{"CANCELLED to DELIVERED", model.StatusCancelled, model.StatusDelivered},
	}

	for _, tc := range invalidCases {
		t.Run(tc.name, func(t *testing.T) {
			err := model.TransitionStatus(tc.current, tc.target)
			if err == nil {
				t.Fatalf("expected transition from %s to %s to fail, but it succeeded", tc.current, tc.target)
			}

			var transitionErr *model.InvalidStateTransitionError
			if !errors.As(err, &transitionErr) {
				t.Fatalf("expected error to be *model.InvalidStateTransitionError, got %T: %v", err, err)
			}

			if transitionErr.CurrentStatus != tc.current {
				t.Errorf("expected CurrentStatus %s, got %s", tc.current, transitionErr.CurrentStatus)
			}
			if transitionErr.TargetStatus != tc.target {
				t.Errorf("expected TargetStatus %s, got %s", tc.target, transitionErr.TargetStatus)
			}
			if transitionErr.HTTPStatusCode() != http.StatusUnprocessableEntity {
				t.Errorf("expected HTTP status 422, got %d", transitionErr.HTTPStatusCode())
			}
		})
	}
}

func TestDeliveryStateMachine_UnknownStatus(t *testing.T) {
	err := model.TransitionStatus("UNKNOWN_STATUS", model.StatusPickedUp)
	if err == nil {
		t.Fatalf("expected error for unknown status, got nil")
	}
	var transitionErr *model.InvalidStateTransitionError
	if !errors.As(err, &transitionErr) {
		t.Fatalf("expected *model.InvalidStateTransitionError, got %T", err)
	}
}

func TestDeliveryStateMachine_IdempotentTransitions(t *testing.T) {
	statuses := []model.DeliveryStatus{
		model.StatusCreated,
		model.StatusPickedUp,
		model.StatusInTransit,
		model.StatusOutForDelivery,
		model.StatusDelivered,
		model.StatusFailed,
		model.StatusCancelled,
	}

	for _, s := range statuses {
		t.Run(string(s)+" to "+string(s), func(t *testing.T) {
			if err := model.TransitionStatus(s, s); err != nil {
				t.Fatalf("expected idempotent transition from %s to %s to succeed, got %v", s, s, err)
			}
		})
	}

	delivery := &model.Delivery{
		ID:     "del-idempotent-test",
		Status: model.StatusPickedUp,
	}
	if err := delivery.TransitionStatus(model.StatusPickedUp); err != nil {
		t.Fatalf("expected delivery.TransitionStatus to same status to succeed, got %v", err)
	}
	if delivery.Status != model.StatusPickedUp {
		t.Fatalf("expected status to remain PICKED_UP, got %s", delivery.Status)
	}
}

