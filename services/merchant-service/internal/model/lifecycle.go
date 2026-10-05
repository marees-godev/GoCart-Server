package model

import (
	"time"

	"github.com/google/uuid"
)

type LifecycleAction string

const (
	LifecycleActionActivate     LifecycleAction = "ACTIVATE"
	LifecycleActionSuspend      LifecycleAction = "SUSPEND"
	LifecycleActionReactivate   LifecycleAction = "REACTIVATE"
	LifecycleActionApprove      LifecycleAction = "APPROVE"
	LifecycleActionReject       LifecycleAction = "REJECT"
	LifecycleActionUpdateStatus LifecycleAction = "UPDATE_STATUS"
)

type AuditStatus string

const (
	AuditStatusSuccess AuditStatus = "SUCCESS"
	AuditStatusFailed  AuditStatus = "FAILED"
)

type MerchantLifecycleAudit struct {
	ID             uuid.UUID       `json:"id" db:"id"`
	MerchantID     uuid.UUID       `json:"merchant_id" db:"merchant_id"`
	AdminID        string          `json:"admin_id" db:"admin_id"`
	Action         LifecycleAction `json:"action" db:"action"`
	PreviousStatus string          `json:"previous_status" db:"previous_status"`
	NewStatus      string          `json:"new_status" db:"new_status"`
	Reason         string          `json:"reason" db:"reason"`
	Status         AuditStatus     `json:"status" db:"status"`
	ErrorMessage   string          `json:"error_message" db:"error_message"`
	RequestID      string          `json:"request_id" db:"request_id"`
	CreatedAt      time.Time       `json:"created_at" db:"created_at"`
}
