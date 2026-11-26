package model

import (
	"testing"
)

func TestCanMerchantCancelOrder(t *testing.T) {
	tests := []struct {
		name          string
		orderStatus   OrderStatus
		paymentStatus PaymentStatus
		expected      bool
	}{
		{"Created with Paid payment", OrderStatusCreated, PaymentStatusPaid, true},
		{"PendingPayment with Paid payment", OrderStatusPendingPayment, PaymentStatusPaid, true},
		{"Paid status with Paid payment", OrderStatusPaid, PaymentStatusPaid, true},
		{"Created with Pending payment", OrderStatusCreated, PaymentStatusPending, false},
		{"PendingPayment with Pending payment", OrderStatusPendingPayment, PaymentStatusPending, false},
		{"Created with Failed payment", OrderStatusCreated, PaymentStatusFailed, false},
		{"Confirmed with Paid payment (not allowed)", OrderStatusConfirmed, PaymentStatusPaid, false},
		{"Processing with Paid payment (not allowed)", OrderStatusProcessing, PaymentStatusPaid, false},
		{"Shipped with Paid payment (not allowed)", OrderStatusShipped, PaymentStatusPaid, false},
		{"Delivered with Paid payment (not allowed)", OrderStatusDelivered, PaymentStatusPaid, false},
		{"Cancelled with Paid payment (not allowed)", OrderStatusCancelled, PaymentStatusPaid, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := CanMerchantCancelOrder(tt.orderStatus, tt.paymentStatus)
			if got != tt.expected {
				t.Errorf("CanMerchantCancelOrder(%s, %s) = %v; want %v", tt.orderStatus, tt.paymentStatus, got, tt.expected)
			}
			// Also verify CanMerchantCancel wrapper
			gotWrapper := CanMerchantCancel(tt.orderStatus, tt.paymentStatus)
			if gotWrapper != tt.expected {
				t.Errorf("CanMerchantCancel(%s, %s) = %v; want %v", tt.orderStatus, tt.paymentStatus, gotWrapper, tt.expected)
			}
		})
	}
}
