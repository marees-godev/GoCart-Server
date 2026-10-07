package model

import (
	"fmt"
	"strings"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

var allowedTransitions = map[ProductStatus][]ProductStatus{
	StatusDraft: {
		StatusPendingReview,
		StatusDeleted,
	},
	StatusPendingReview: {
		StatusPublished,
		StatusUnpublished,
		StatusSuspended,
		StatusDeleted,
		StatusDraft,
	},
	StatusPublished: {
		StatusUnpublished,
		StatusSuspended,
		StatusDeleted,
	},
	StatusUnpublished: {
		StatusPendingReview,
		StatusPublished,
		StatusSuspended,
		StatusDeleted,
		StatusDraft,
	},
	StatusSuspended: {
		StatusPendingReview,
		StatusUnpublished,
		StatusPublished,
		StatusDeleted,
	},
	StatusDeleted: {},

	// Compatibility for legacy statuses
	StatusInStock:      {StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted},
	StatusOutOfStock:   {StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted},
	StatusLowStock:     {StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted},
	StatusReserved:     {StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted},
	StatusDiscontinued: {StatusDraft, StatusPendingReview, StatusPublished, StatusUnpublished, StatusSuspended, StatusDeleted},
}

type InvalidStateTransitionError struct {
	FromStatus ProductStatus
	ToStatus   ProductStatus
	Allowed    []ProductStatus
}

func (e *InvalidStateTransitionError) Error() string {
	return fmt.Sprintf("Cannot transition product from '%s' to '%s'. Allowed targets: %s.", e.FromStatus, e.ToStatus, FormatAllowedTargets(e.Allowed))
}

func FormatAllowedTargets(targets []ProductStatus) string {
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

func (v *StateTransitionValidator) Validate(from, to ProductStatus) error {
	if from == to {
		return nil
	}
	targets, exists := allowedTransitions[from]
	if !exists {
		return appErrors.Conflict(fmt.Sprintf("Cannot transition product from '%s' to '%s'. Allowed targets: [].", from, to))
	}

	for _, allowed := range targets {
		if allowed == to {
			return nil
		}
	}

	return appErrors.Conflict(fmt.Sprintf("Cannot transition product from '%s' to '%s'. Allowed targets: %s.", from, to, FormatAllowedTargets(targets)))
}

func (v *StateTransitionValidator) AllowedTargets(from ProductStatus) []ProductStatus {
	if targets, ok := allowedTransitions[from]; ok {
		return targets
	}
	return []ProductStatus{}
}
