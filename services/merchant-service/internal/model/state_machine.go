package model

import (
	"fmt"
	"strings"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)


var allowedTransitions = map[MerchantStatus][]MerchantStatus{
	MerchantStatusPending:       {MerchantStatusApproved, MerchantStatusRejected, MerchantStatusActive},
	MerchantStatusPendingReview: {MerchantStatusActive, MerchantStatusRejected},
	MerchantStatusInactive:      {MerchantStatusActive},
	MerchantStatusApproved:      {MerchantStatusSuspended, MerchantStatusActive},
	MerchantStatusActive:        {MerchantStatusSuspended, MerchantStatusInactive, MerchantStatusTerminated},
	MerchantStatusSuspended:     {MerchantStatusActive, MerchantStatusPending, MerchantStatusTerminated},
	MerchantStatusRejected:      {},
	MerchantStatusTerminated:    {},
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
	case MerchantStatusPending, MerchantStatusApproved, MerchantStatusRejected, MerchantStatusSuspended,
		MerchantStatusActive, MerchantStatusInactive, MerchantStatusPendingReview, MerchantStatusTerminated:
		return true
	default:
		return false
	}
}

// ValidateLifecycleTransition enforces state machine rules for administrative lifecycle operations:
// ActivateMerchant, SuspendMerchant, and ReactivateMerchant.
// It returns the target status on success, or a 409 Conflict AppError on conflict/disallowed transition.
func ValidateLifecycleTransition(action LifecycleAction, currentStatus MerchantStatus) (MerchantStatus, error) {
	curr := MerchantStatus(strings.ToUpper(strings.TrimSpace(string(currentStatus))))

	switch action {
	case LifecycleActionActivate:
		if curr == MerchantStatusActive {
			return "", appErrors.Conflict("merchant is already active")
		}
		if curr == MerchantStatusTerminated {
			return "", appErrors.Conflict("cannot activate a terminated merchant")
		}
		if curr == MerchantStatusSuspended {
			return "", appErrors.Conflict("suspended merchant must be reactivated, not activated")
		}
		if curr == MerchantStatusPending || curr == MerchantStatusPendingReview || curr == MerchantStatusInactive || curr == MerchantStatusApproved {
			return MerchantStatusActive, nil
		}
		return "", appErrors.Conflict(fmt.Sprintf("cannot activate merchant from status '%s'", curr))

	case LifecycleActionSuspend:
		if curr == MerchantStatusSuspended {
			return "", appErrors.Conflict("merchant is already suspended")
		}
		if curr == MerchantStatusTerminated {
			return "", appErrors.Conflict("cannot suspend a terminated merchant")
		}
		if curr == MerchantStatusActive || curr == MerchantStatusApproved {
			return MerchantStatusSuspended, nil
		}
		return "", appErrors.Conflict(fmt.Sprintf("cannot suspend merchant from status '%s'; only active merchants can be suspended", curr))

	case LifecycleActionReactivate:
		if curr == MerchantStatusActive {
			return "", appErrors.Conflict("merchant is already active")
		}
		if curr == MerchantStatusTerminated {
			return "", appErrors.Conflict("cannot reactivate a terminated merchant")
		}
		if curr == MerchantStatusSuspended {
			return MerchantStatusActive, nil
		}
		return "", appErrors.Conflict(fmt.Sprintf("merchant is not suspended (current status: '%s')", curr))

	default:
		return "", appErrors.BadRequest(fmt.Sprintf("unsupported lifecycle action: %s", action))
	}
}
