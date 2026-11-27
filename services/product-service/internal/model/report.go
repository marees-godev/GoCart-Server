package model

import (
	"strings"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type ReportReason string

const (
	ReportReasonInappropriateContent    ReportReason = "INAPPROPRIATE_CONTENT"
	ReportReasonMisleadingInformation   ReportReason = "MISLEADING_INFORMATION"
	ReportReasonCounterfeitOrFraudulent ReportReason = "COUNTERFEIT_OR_FRAUDULENT"
	ReportReasonProhibitedProduct       ReportReason = "PROHIBITED_PRODUCT"
	ReportReasonSpamOrDuplicate         ReportReason = "SPAM_OR_DUPLICATE"
	ReportReasonOther                   ReportReason = "OTHER"
)

type ReportStatus string

const (
	ReportStatusPending     ReportStatus = "PENDING"
	ReportStatusReviewed    ReportStatus = "REVIEWED"
	ReportStatusDismissed   ReportStatus = "DISMISSED"
	ReportStatusActionTaken ReportStatus = "ACTION_TAKEN"
)

func NormalizeReportReason(raw string) (ReportReason, error) {
	clean := strings.ToUpper(strings.TrimSpace(raw))
	clean = strings.ReplaceAll(clean, " ", "_")
	clean = strings.ReplaceAll(clean, "-", "_")

	switch clean {
	case "INAPPROPRIATE_CONTENT", "INAPPROPRIATE":
		return ReportReasonInappropriateContent, nil
	case "MISLEADING_INFORMATION", "MISLEADING", "MISLEADING_PRODUCT_INFORMATION":
		return ReportReasonMisleadingInformation, nil
	case "COUNTERFEIT_OR_FRAUDULENT", "COUNTERFEIT", "FRAUDULENT", "COUNTERFEIT_PRODUCT", "FRAUDULENT_PRODUCT":
		return ReportReasonCounterfeitOrFraudulent, nil
	case "PROHIBITED_PRODUCT", "PROHIBITED":
		return ReportReasonProhibitedProduct, nil
	case "SPAM_OR_DUPLICATE", "SPAM", "DUPLICATE", "SPAM_OR_DUPLICATE_LISTING":
		return ReportReasonSpamOrDuplicate, nil
	case "OTHER":
		return ReportReasonOther, nil
	default:
		return "", appErrors.BadRequest("invalid report reason: must be one of INAPPROPRIATE_CONTENT, MISLEADING_INFORMATION, COUNTERFEIT_OR_FRAUDULENT, PROHIBITED_PRODUCT, SPAM_OR_DUPLICATE, OTHER")
	}
}

func (r ReportReason) IsValid() bool {
	switch r {
	case ReportReasonInappropriateContent,
		ReportReasonMisleadingInformation,
		ReportReasonCounterfeitOrFraudulent,
		ReportReasonProhibitedProduct,
		ReportReasonSpamOrDuplicate,
		ReportReasonOther:
		return true
	default:
		return false
	}
}

func (s ReportStatus) IsValid() bool {
	switch s {
	case ReportStatusPending,
		ReportStatusReviewed,
		ReportStatusDismissed,
		ReportStatusActionTaken:
		return true
	default:
		return false
	}
}

type ProductReport struct {
	ID          string       `json:"id" db:"id"`
	ProductID   string       `json:"product_id" db:"product_id"`
	ReporterID  string       `json:"reporter_id" db:"reporter_id"`
	Reason      ReportReason `json:"reason" db:"reason"`
	Description string       `json:"description,omitempty" db:"description"`
	Status      ReportStatus `json:"status" db:"status"`
	CreatedAt   time.Time    `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time    `json:"updated_at" db:"updated_at"`
}
