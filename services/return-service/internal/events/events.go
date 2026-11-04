package events

import (
	"fmt"
	"time"

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

func BuildReturnApprovedOutboxEvent(ret *model.Return, reviewerID string) (*outbox.Event, error) {
	approvedAt := ret.UpdatedAt
	if ret.ApprovedAt != nil {
		approvedAt = *ret.ApprovedAt
	}
	payload := events.ReturnApprovedEvent{
		ReturnID:     ret.ID,
		OrderID:      ret.OrderID,
		OrderItemID:  ret.OrderItemID,
		UserID:       ret.UserID,
		ApprovedBy:   reviewerID,
		ApprovedAt:   approvedAt,
		Type:         string(ret.Type),
		RefundAmount: ret.RefundAmount,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnApproved, SourceReturnService, ret.ID, payload)
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
		EventType:     events.EventTypeReturnApproved,
		Payload:       bytes,
		Topic:         events.TopicReturnApproved,
	}, nil
}

func BuildReturnRejectedOutboxEvent(ret *model.Return, reviewerID string, reason string) (*outbox.Event, error) {
	payload := events.ReturnRejectedEvent{
		ReturnID:        ret.ID,
		OrderID:         ret.OrderID,
		OrderItemID:     ret.OrderItemID,
		UserID:          ret.UserID,
		RejectedBy:      reviewerID,
		RejectionReason: reason,
		RejectedAt:      ret.UpdatedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnRejected, SourceReturnService, ret.ID, payload)
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
		EventType:     events.EventTypeReturnRejected,
		Payload:       bytes,
		Topic:         events.TopicReturnRejected,
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

func BuildReturnRefundEligibleOutboxEvent(ret *model.Return, reason string) (*outbox.Event, error) {
	if reason == "" {
		reason = fmt.Sprintf("Return %s verified and eligible for refund", ret.ID)
	}
	payload := events.ReturnRefundEligibleEvent{
		ReturnID:     ret.ID,
		OrderID:      ret.OrderID,
		OrderItemID:  ret.OrderItemID,
		UserID:       ret.UserID,
		Type:         string(ret.Type),
		RefundAmount: ret.RefundAmount,
		Reason:       reason,
		EligibleAt:   time.Now().UTC(),
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnRefundEligible, SourceReturnService, ret.ID, payload)
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
		EventType:     events.EventTypeReturnRefundEligible,
		Payload:       bytes,
		Topic:         events.TopicReturnRefundEligible,
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

func BuildExchangeCreatedOutboxEvent(exc *model.Exchange) (*outbox.Event, error) {
	payload := map[string]interface{}{
		"exchange_id":         exc.ID,
		"order_id":            exc.OrderID,
		"order_item_id":       exc.OrderItemID,
		"user_id":             exc.UserID,
		"new_product_id":      exc.NewProductID,
		"old_brand":           exc.OldBrand,
		"old_model":           exc.OldModel,
		"old_variant":         exc.OldVariant,
		"serial_or_imei":      exc.SerialOrIMEI,
		"base_exchange_value": exc.BaseExchangeValue,
		"estimated_discount":  exc.EstimatedDiscount,
		"status":              string(exc.Status),
		"created_at":          exc.CreatedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeExchangeCreated, SourceReturnService, exc.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "exchange",
		AggregateID:   exc.ID,
		EventType:     events.EventTypeExchangeCreated,
		Payload:       bytes,
		Topic:         events.TopicExchangeCreated,
	}, nil
}

func BuildExchangeOfferAppliedOutboxEvent(exc *model.Exchange) (*outbox.Event, error) {
	payload := events.ExchangeOfferAppliedEvent{
		ExchangeID:        exc.ID,
		QuoteID:           exc.QuoteID,
		OrderID:           exc.OrderID,
		OrderItemID:       exc.OrderItemID,
		UserID:            exc.UserID,
		NewProductID:      exc.NewProductID,
		OldBrand:          exc.OldBrand,
		OldModel:          exc.OldModel,
		OldVariant:        exc.OldVariant,
		SerialOrIMEI:      exc.SerialOrIMEI,
		BaseExchangeValue: exc.BaseExchangeValue,
		DiscountAmount:    exc.EstimatedDiscount,
		AppliedAt:         exc.CreatedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeExchangeOfferApplied, SourceReturnService, exc.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "exchange",
		AggregateID:   exc.ID,
		EventType:     events.EventTypeExchangeOfferApplied,
		Payload:       bytes,
		Topic:         events.TopicExchangeOfferApplied,
	}, nil
}

func BuildExchangeCompletedOutboxEvent(exc *model.Exchange) (*outbox.Event, error) {
	payload := events.ExchangeCompletedEvent{
		ExchangeID:    exc.ID,
		OrderID:       exc.OrderID,
		OrderItemID:   exc.OrderItemID,
		UserID:        exc.UserID,
		FinalDiscount: exc.FinalDiscount,
		CompletedAt:   exc.UpdatedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeExchangeCompleted, SourceReturnService, exc.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "exchange",
		AggregateID:   exc.ID,
		EventType:     events.EventTypeExchangeCompleted,
		Payload:       bytes,
		Topic:         events.TopicExchangeCompleted,
	}, nil
}

func BuildExchangeVerifiedOutboxEvent(exc *model.Exchange, v *model.ExchangeVerification) (*outbox.Event, error) {
	payload := map[string]interface{}{
		"exchange_id":       exc.ID,
		"order_id":          exc.OrderID,
		"verified_by":       v.VerifiedBy,
		"condition_rating":  string(v.ConditionRating),
		"is_approved":       v.IsApproved,
		"screen_intact":     v.ScreenIntact,
		"body_intact":       v.BodyIntact,
		"functional":        v.Functional,
		"serial_matches":    v.SerialMatches,
		"adjusted_discount": v.AdjustedDiscount,
		"notes":             v.Notes,
		"verified_at":       v.VerifiedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeExchangeVerified, SourceReturnService, exc.ID, payload)
	if err != nil {
		return nil, err
	}

	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}

	return &outbox.Event{
		AggregateType: "exchange",
		AggregateID:   exc.ID,
		EventType:     events.EventTypeExchangeVerified,
		Payload:       bytes,
		Topic:         events.TopicExchangeVerified,
	}, nil
}

func BuildReturnCompletedOutboxEvent(ret *model.Return) (*outbox.Event, error) {
	completedAt := ret.UpdatedAt
	if ret.CompletedAt != nil {
		completedAt = *ret.CompletedAt
	}
	payload := events.ReturnCompletedEvent{
		ReturnID:     ret.ID,
		OrderID:      ret.OrderID,
		OrderItemID:  ret.OrderItemID,
		UserID:       ret.UserID,
		RefundAmount: ret.RefundAmount,
		CompletedAt:  completedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeReturnCompleted, SourceReturnService, ret.ID, payload)
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
		EventType:     events.EventTypeReturnCompleted,
		Payload:       bytes,
		Topic:         events.TopicReturnCompleted,
	}, nil
}

func BuildRefundInitiatedOutboxEvent(ret *model.Return, paymentID string, amount float64, currency string) (*outbox.Event, error) {
	if currency == "" {
		currency = "INR"
	}
	now := time.Now().UTC()
	payload := events.RefundInitiatedEvent{
		RefundID:    fmt.Sprintf("ref-%s", ret.ID),
		PaymentID:   paymentID,
		OrderID:     ret.OrderID,
		Amount:      amount,
		Currency:    currency,
		Reason:      ret.Reason,
		InitiatedAt: now,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeRefundInitiated, SourceReturnService, ret.ID, payload)
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
		EventType:     events.EventTypeRefundInitiated,
		Payload:       bytes,
		Topic:         events.TopicRefundInitiated,
	}, nil
}
