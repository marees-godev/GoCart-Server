package model_test

import (
	"testing"

	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
)

func TestCanTransitionPayment(t *testing.T) {
	tests := []struct {
		name     string
		from     model.PaymentStatus
		to       model.PaymentStatus
		expected bool
	}{
		{"Initiated to Pending", model.PaymentStatusInitiated, model.PaymentStatusPending, true},
		{"Initiated to Success", model.PaymentStatusInitiated, model.PaymentStatusSuccess, true},
		{"Initiated to Failed", model.PaymentStatusInitiated, model.PaymentStatusFailed, true},
		{"Pending to Success", model.PaymentStatusPending, model.PaymentStatusSuccess, true},
		{"Pending to Failed", model.PaymentStatusPending, model.PaymentStatusFailed, true},
		{"Success to Refunded", model.PaymentStatusSuccess, model.PaymentStatusRefunded, true},
		{"Duplicate Success", model.PaymentStatusSuccess, model.PaymentStatusSuccess, false},
		{"Duplicate Failed", model.PaymentStatusFailed, model.PaymentStatusFailed, false},
		{"Duplicate Pending", model.PaymentStatusPending, model.PaymentStatusPending, false},
		{"Success to Failed (Invalid)", model.PaymentStatusSuccess, model.PaymentStatusFailed, false},
		{"Success to Pending (Invalid)", model.PaymentStatusSuccess, model.PaymentStatusPending, false},
		{"Failed to Success (Invalid)", model.PaymentStatusFailed, model.PaymentStatusSuccess, false},
		{"Failed to Pending (Invalid)", model.PaymentStatusFailed, model.PaymentStatusPending, false},
		{"Refunded to Success (Invalid)", model.PaymentStatusRefunded, model.PaymentStatusSuccess, false},
		{"Cancelled to Success (Invalid)", model.PaymentStatusCancelled, model.PaymentStatusSuccess, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := model.CanTransitionPayment(tt.from, tt.to)
			if got != tt.expected {
				t.Errorf("CanTransitionPayment(%s, %s) = %v; want %v", tt.from, tt.to, got, tt.expected)
			}
		})
	}
}

func TestIsTerminalPaymentStatus(t *testing.T) {
	if !model.IsTerminalPaymentStatus(model.PaymentStatusSuccess) {
		t.Errorf("expected SUCCESS to be terminal for charging")
	}
	if !model.IsTerminalPaymentStatus(model.PaymentStatusFailed) {
		t.Errorf("expected FAILED to be terminal")
	}
	if !model.IsTerminalPaymentStatus(model.PaymentStatusCancelled) {
		t.Errorf("expected CANCELLED to be terminal")
	}
	if !model.IsTerminalPaymentStatus(model.PaymentStatusRefunded) {
		t.Errorf("expected REFUNDED to be terminal")
	}
	if model.IsTerminalPaymentStatus(model.PaymentStatusInitiated) {
		t.Errorf("expected INITIATED to not be terminal")
	}
	if model.IsTerminalPaymentStatus(model.PaymentStatusPending) {
		t.Errorf("expected PENDING to not be terminal")
	}
}
