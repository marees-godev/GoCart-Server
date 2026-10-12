package model_test

import (
	"testing"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

func TestProductStateMachineTransitions(t *testing.T) {
	validator := model.NewStateTransitionValidator()

	tests := []struct {
		name      string
		from      model.ProductStatus
		to        model.ProductStatus
		expectErr bool
	}{
		{"DRAFT -> PENDING_REVIEW (Valid)", model.StatusDraft, model.StatusPendingReview, false},
		{"DRAFT -> DELETED (Valid)", model.StatusDraft, model.StatusDeleted, false},
		{"DRAFT -> PUBLISHED (Invalid)", model.StatusDraft, model.StatusPublished, true},
		{"DRAFT -> SUSPENDED (Invalid)", model.StatusDraft, model.StatusSuspended, true},

		{"PENDING_REVIEW -> PUBLISHED (Valid)", model.StatusPendingReview, model.StatusPublished, false},
		{"PENDING_REVIEW -> UNPUBLISHED (Valid)", model.StatusPendingReview, model.StatusUnpublished, false},
		{"PENDING_REVIEW -> SUSPENDED (Valid)", model.StatusPendingReview, model.StatusSuspended, false},
		{"PENDING_REVIEW -> DELETED (Valid)", model.StatusPendingReview, model.StatusDeleted, false},

		{"PUBLISHED -> UNPUBLISHED (Valid)", model.StatusPublished, model.StatusUnpublished, false},
		{"PUBLISHED -> SUSPENDED (Valid)", model.StatusPublished, model.StatusSuspended, false},
		{"PUBLISHED -> DELETED (Valid)", model.StatusPublished, model.StatusDeleted, false},
		{"PUBLISHED -> PENDING_REVIEW (Invalid)", model.StatusPublished, model.StatusPendingReview, true},

		{"UNPUBLISHED -> PUBLISHED (Valid)", model.StatusUnpublished, model.StatusPublished, false},
		{"UNPUBLISHED -> PENDING_REVIEW (Valid)", model.StatusUnpublished, model.StatusPendingReview, false},
		{"UNPUBLISHED -> SUSPENDED (Valid)", model.StatusUnpublished, model.StatusSuspended, false},
		{"UNPUBLISHED -> DELETED (Valid)", model.StatusUnpublished, model.StatusDeleted, false},

		{"SUSPENDED -> PUBLISHED (Valid)", model.StatusSuspended, model.StatusPublished, false},
		{"SUSPENDED -> PENDING_REVIEW (Valid)", model.StatusSuspended, model.StatusPendingReview, false},
		{"SUSPENDED -> UNPUBLISHED (Valid)", model.StatusSuspended, model.StatusUnpublished, false},
		{"SUSPENDED -> DELETED (Valid)", model.StatusSuspended, model.StatusDeleted, false},

		{"DELETED -> PUBLISHED (Invalid)", model.StatusDeleted, model.StatusPublished, true},
		{"DELETED -> DRAFT (Invalid)", model.StatusDeleted, model.StatusDraft, true},

		{"Self transition PUBLISHED -> PUBLISHED (Valid)", model.StatusPublished, model.StatusPublished, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validator.Validate(tt.from, tt.to)
			if tt.expectErr && err == nil {
				t.Fatalf("expected error transitioning from %s to %s, got nil", tt.from, tt.to)
			}
			if !tt.expectErr && err != nil {
				t.Fatalf("unexpected error transitioning from %s to %s: %v", tt.from, tt.to, err)
			}
			if tt.expectErr {
				appErr := appErrors.AsAppError(err)
				if appErr == nil || appErr.Code != appErrors.CodeConflict {
					t.Fatalf("expected conflict error, got %v", err)
				}
			}
		})
	}
}

func TestProductValidateForPublishing(t *testing.T) {
	validProduct := &model.Product{
		Name:       "Test Product",
		Price:      100.0,
		CategoryID: "cat-123",
		ImageURL:   "https://example.com/image.jpg",
	}

	if err := validProduct.ValidateForPublishing(); err != nil {
		t.Fatalf("expected valid product for publishing, got %v", err)
	}

	missingName := &model.Product{Price: 100, CategoryID: "cat-123", ImageURL: "https://example.com/img.jpg"}
	if err := missingName.ValidateForPublishing(); err == nil {
		t.Fatal("expected error for missing name")
	}

	invalidPrice := &model.Product{Name: "Test", Price: 0, CategoryID: "cat-123", ImageURL: "https://example.com/img.jpg"}
	if err := invalidPrice.ValidateForPublishing(); err == nil {
		t.Fatal("expected error for zero price")
	}

	missingCat := &model.Product{Name: "Test", Price: 100, ImageURL: "https://example.com/img.jpg"}
	if err := missingCat.ValidateForPublishing(); err == nil {
		t.Fatal("expected error for missing category")
	}

	missingImage := &model.Product{Name: "Test", Price: 100, CategoryID: "cat-123"}
	if err := missingImage.ValidateForPublishing(); err == nil {
		t.Fatal("expected error for missing image")
	}
}
