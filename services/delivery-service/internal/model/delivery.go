package model

import (
	"database/sql/driver"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
)

type DeliveryStatus string

const (
	StatusCreated        DeliveryStatus = "CREATED"
	StatusPickedUp       DeliveryStatus = "PICKED_UP"
	StatusInTransit      DeliveryStatus = "IN_TRANSIT"
	StatusOutForDelivery DeliveryStatus = "OUT_FOR_DELIVERY"
	StatusDelivered      DeliveryStatus = "DELIVERED"
	StatusFailed         DeliveryStatus = "FAILED"
	StatusCancelled      DeliveryStatus = "CANCELLED"
)

var AllowedTransitions = map[DeliveryStatus][]DeliveryStatus{
	StatusCreated:        {StatusPickedUp, StatusCancelled, StatusFailed},
	StatusPickedUp:       {StatusInTransit, StatusFailed},
	StatusInTransit:      {StatusOutForDelivery, StatusFailed},
	StatusOutForDelivery: {StatusDelivered, StatusFailed},
	StatusDelivered:      {},
	StatusFailed:         {},
	StatusCancelled:      {},
}

func IsValidStatus(status DeliveryStatus) bool {
	clean := DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(status))))
	switch clean {
	case StatusCreated, StatusPickedUp, StatusInTransit, StatusOutForDelivery, StatusDelivered, StatusFailed, StatusCancelled:
		return true
	default:
		return false
	}
}

type InvalidStateTransitionError struct {
	CurrentStatus DeliveryStatus `json:"current_status"`
	TargetStatus  DeliveryStatus `json:"target_status"`
	Message       string         `json:"message"`
}

func (e *InvalidStateTransitionError) Error() string {
	if e.Message != "" {
		return e.Message
	}
	return fmt.Sprintf("invalid delivery state transition from %s to %s", e.CurrentStatus, e.TargetStatus)
}

func (e *InvalidStateTransitionError) HTTPStatusCode() int {
	return http.StatusUnprocessableEntity
}

func (e *InvalidStateTransitionError) ToAppError() *appErrors.AppError {
	return appErrors.New(appErrors.CodeUnprocessableEntity, e.Error(), http.StatusUnprocessableEntity)
}

func TransitionStatus(currentStatus, targetStatus DeliveryStatus) error {
	current := DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(currentStatus))))
	target := DeliveryStatus(strings.ToUpper(strings.TrimSpace(string(targetStatus))))

	if current == target {
		return nil
	}

	allowed, ok := AllowedTransitions[current]
	if !ok {
		return &InvalidStateTransitionError{
			CurrentStatus: current,
			TargetStatus:  target,
			Message:       fmt.Sprintf("unknown or invalid current status: %s", current),
		}
	}

	if len(allowed) == 0 {
		return &InvalidStateTransitionError{
			CurrentStatus: current,
			TargetStatus:  target,
			Message:       fmt.Sprintf("cannot transition from terminal state %s to %s", current, target),
		}
	}

	for _, s := range allowed {
		if s == target {
			return nil
		}
	}

	return &InvalidStateTransitionError{
		CurrentStatus: current,
		TargetStatus:  target,
		Message:       fmt.Sprintf("invalid delivery state transition from %s to %s", current, target),
	}
}

type ShippingAddress struct {
	Street        string `json:"street"`
	City          string `json:"city"`
	State         string `json:"state"`
	PostalCode    string `json:"postal_code"`
	Country       string `json:"country"`
	ContactNumber string `json:"contact_number"`
}

func (a ShippingAddress) Value() (driver.Value, error) {
	return json.Marshal(a)
}

func (a *ShippingAddress) Scan(value any) error {
	if value == nil {
		return nil
	}
	switch v := value.(type) {
	case []byte:
		return json.Unmarshal(v, a)
	case string:
		return json.Unmarshal([]byte(v), a)
	default:
		return fmt.Errorf("unsupported type for ShippingAddress scan: %T", value)
	}
}

type Delivery struct {
	ID              string          `json:"id"`
	OrderID         string          `json:"order_id"`
	DeliveryPartner string          `json:"delivery_partner"`
	TrackingNumber  string          `json:"tracking_number"`
	ShippingAddress ShippingAddress `json:"shipping_address"`
	Status            DeliveryStatus  `json:"status"`
	EstimatedDelivery *time.Time      `json:"estimated_delivery,omitempty"`
	ActualDelivery    *time.Time      `json:"actual_delivery,omitempty"`
	CreatedAt         time.Time       `json:"created_at"`
	UpdatedAt         time.Time       `json:"updated_at"`
}

func (d *Delivery) TransitionStatus(target DeliveryStatus) error {
	if err := TransitionStatus(d.Status, target); err != nil {
		return err
	}
	d.Status = target
	d.UpdatedAt = time.Now().UTC()
	return nil
}
