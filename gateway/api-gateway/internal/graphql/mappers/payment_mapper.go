package maps

import (
	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapPayment(p *paymentpb.Payment) *model.Payment {
	if p == nil {
		return nil
	}

	var txRef *string
	if p.TransactionReference != "" {
		txRef = &p.TransactionReference
	}

	var gtxID *string
	if p.GatewayTransactionId != "" {
		gtxID = &p.GatewayTransactionId
	}

	var userID *string
	if p.UserId != "" {
		userID = &p.UserId
	}

	var idempKey *string
	if p.IdempotencyKey != "" {
		idempKey = &p.IdempotencyKey
	}

	var failReason *string
	if p.FailureReason != "" {
		failReason = &p.FailureReason
	}

	var createdAt *string
	if p.CreatedAt != "" {
		createdAt = &p.CreatedAt
	}

	var updatedAt *string
	if p.UpdatedAt != "" {
		updatedAt = &p.UpdatedAt
	}

	return &model.Payment{
		ID:                   p.Id,
		OrderID:              p.OrderId,
		UserID:               userID,
		Amount:               p.Amount,
		Currency:             p.Currency,
		PaymentMethod:        p.PaymentMethod,
		Status:               p.Status,
		TransactionReference: txRef,
		GatewayTransactionID: gtxID,
		IdempotencyKey:       idempKey,
		FailureReason:        failReason,
		CreatedAt:            createdAt,
		UpdatedAt:            updatedAt,
	}
}

func MapRefundPayload(r *paymentpb.CreateRefundResponse) *model.RefundPayload {
	if r == nil {
		return nil
	}

	var gwRefundID *string
	if r.GatewayRefundId != "" {
		gwRefundID = &r.GatewayRefundId
	}

	var createdAt *string
	if r.CreatedAt != "" {
		createdAt = &r.CreatedAt
	}

	return &model.RefundPayload{
		RefundID:        r.RefundId,
		Status:          r.Status,
		Amount:          r.Amount,
		GatewayRefundID: gwRefundID,
		CreatedAt:       createdAt,
	}
}
