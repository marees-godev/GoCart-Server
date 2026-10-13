package model

import (
	"fmt"
)

var allowedPaymentTransitions = map[PaymentStatus][]PaymentStatus{
	PaymentStatusInitiated: {PaymentStatusPending, PaymentStatusSuccess, PaymentStatusFailed, PaymentStatusCancelled},
	PaymentStatusPending:   {PaymentStatusSuccess, PaymentStatusFailed, PaymentStatusCancelled},
	PaymentStatusSuccess:   {PaymentStatusRefunded},
	PaymentStatusFailed:    {},
	PaymentStatusCancelled: {},
	PaymentStatusRefunded:  {},
}

func CanTransitionPayment(from, to PaymentStatus) bool {
	if from == to {
		return false
	}
	allowed, exists := allowedPaymentTransitions[from]
	if !exists {
		return false
	}
	for _, target := range allowed {
		if target == to {
			return true
		}
	}
	return false
}

func IsTerminalPaymentStatus(status PaymentStatus) bool {
	switch status {
	case PaymentStatusSuccess, PaymentStatusFailed, PaymentStatusCancelled, PaymentStatusRefunded:
		return true
	default:
		return false
	}
}

type InvalidPaymentTransitionError struct {
	From PaymentStatus
	To   PaymentStatus
}

func (e *InvalidPaymentTransitionError) Error() string {
	return fmt.Sprintf("cannot transition payment from '%s' to '%s'", e.From, e.To)
}
