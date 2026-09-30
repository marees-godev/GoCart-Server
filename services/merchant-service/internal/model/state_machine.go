package model

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

var allowedTransitions = map[MerchantStatus][]MerchantStatus{
	MerchantStatusPending:   {MerchantStatusApproved, MerchantStatusRejected},
	MerchantStatusApproved:  {MerchantStatusSuspended},
	MerchantStatusRejected:  {},
	MerchantStatusSuspended: {},
}

type InvalidStateTransitionError struct {
	FromStatus MerchantStatus
	ToStatus   MerchantStatus
	Allowed    []MerchantStatus
}

func (e *InvalidStateTransitionError) Error() string {
	return fmt.Sprintf("Cannot transition merchant from '%s' to '%s'. Allowed targets: %s.", e.FromStatus, e.ToStatus, FormatAllowedTargets(e.Allowed))
}

func FormatAllowedTargets(targets []MerchantStatus) string {
	if len(targets) == 0 {
		return "[]"
	}
	formatted := make([]string, len(targets))
	for i, t := range targets {
		formatted[i] = fmt.Sprintf("'%s'", t)
	}
	return "[" + strings.Join(formatted, ", ") + "]"
}

type StateTransitionValidator struct{}

func NewStateTransitionValidator() *StateTransitionValidator {
	return &StateTransitionValidator{}
}

func (v *StateTransitionValidator) Validate(from, to MerchantStatus) error {
	targets, exists := allowedTransitions[from]
	if !exists {
		return &InvalidStateTransitionError{
			FromStatus: from,
			ToStatus:   to,
			Allowed:    []MerchantStatus{},
		}
	}

	for _, allowed := range targets {
		if allowed == to {
			return nil
		}
	}

	return &InvalidStateTransitionError{
		FromStatus: from,
		ToStatus:   to,
		Allowed:    targets,
	}
}

func (v *StateTransitionValidator) AllowedTargets(from MerchantStatus) []MerchantStatus {
	if targets, ok := allowedTransitions[from]; ok {
		return targets
	}
	return []MerchantStatus{}
}

func IsValidStatus(status string) bool {
	switch MerchantStatus(strings.ToUpper(strings.TrimSpace(status))) {
	case MerchantStatusPending, MerchantStatusApproved, MerchantStatusRejected, MerchantStatusSuspended:
		return true
	default:
		return false
	}
}

type MerchantStatusAudit struct {
	ID         uuid.UUID      `json:"id" db:"id"`
	MerchantID uuid.UUID      `json:"merchant_id" db:"merchant_id"`
	FromStatus MerchantStatus `json:"from_status" db:"from_status"`
	ToStatus   MerchantStatus `json:"to_status" db:"to_status"`
	Reason     string         `json:"reason" db:"reason"`
	UpdatedBy  string         `json:"updated_by" db:"updated_by"`
	CreatedAt  time.Time      `json:"created_at" db:"created_at"`
}
