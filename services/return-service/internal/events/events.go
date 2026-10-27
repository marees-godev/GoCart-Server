package events

import (
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
)

const SourceReturnService = "return-service"

func BuildReturnRequestedOutboxEvent(ret *model.Return, productID string) (*outbox.Event, error) {
	payload := events.ReturnRequestedEvent{
		ReturnID:     ret.ID,
		OrderID:      ret.OrderID,
		OrderItemID:  ret.OrderItemID,
		ProductID:    productID,
		UserID:       ret.UserID,
		Reason:       ret.Reason,
		Status:       string(ret.Status),
		Type:         string(ret.Type),
		RefundAmount: ret.RefundAmount,
		RequestedAt:  ret.RequestedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnRequested, SourceReturnService, ret.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "return",
		AggregateID:   ret.ID,
		EventType:     events.EventTypeReturnRequested,
		Payload:       bytes,
		Topic:         events.TopicReturnRequested,
	}, nil
}

func BuildReturnVerifiedOutboxEvent(ret *model.Return, v *model.ProductVerification) (*outbox.Event, error) {
	payload := events.ReturnVerifiedEvent{
		ReturnID:    ret.ID,
		OrderID:     ret.OrderID,
		OrderItemID: ret.OrderItemID,
		VerifiedBy:  v.VerifiedBy,
		Condition:   string(v.Condition),
		IsApproved:  v.IsApproved,
		ProofImages: v.ProofImages,
		Notes:       v.Notes,
		VerifiedAt:  v.VerifiedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnVerified, SourceReturnService, ret.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "return",
		AggregateID:   ret.ID,
		EventType:     events.EventTypeReturnVerified,
		Payload:       bytes,
		Topic:         events.TopicReturnVerified,
	}, nil
}

func BuildReturnStatusUpdatedOutboxEvent(ret *model.Return, prevStatus, newStatus model.ReturnStatus) (*outbox.Event, error) {
	payload := events.ReturnStatusUpdatedEvent{
		ReturnID:       ret.ID,
		OrderID:        ret.OrderID,
		OrderItemID:    ret.OrderItemID,
		PreviousStatus: string(prevStatus),
		NewStatus:      string(newStatus),
		UpdatedAt:      ret.UpdatedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnStatusUpdated, SourceReturnService, ret.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "return",
		AggregateID:   ret.ID,
		EventType:     events.EventTypeReturnStatusUpdated,
		Payload:       bytes,
		Topic:         events.TopicReturnStatusUpdated,
	}, nil
}

