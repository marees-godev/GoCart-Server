package maps

import (
	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
)

func MapReturnOrder(protoReturn *returnpb.ReturnOrder) *model.ReturnOrder {
	if protoReturn == nil {
		return nil
	}

	res := &model.ReturnOrder{
		ID:           protoReturn.Id,
		OrderID:      protoReturn.OrderId,
		UserID:       protoReturn.UserId,
		Reason:       protoReturn.Reason,
		Status:       protoReturn.Status,
		Type:         protoReturn.Type,
		RefundAmount: protoReturn.RefundAmount,
	}

	if protoReturn.OrderItemId != "" {
		res.OrderItemID = &protoReturn.OrderItemId
	}
	if protoReturn.RequestedAt != "" {
		res.RequestedAt = &protoReturn.RequestedAt
	}
	if protoReturn.ApprovedAt != "" {
		res.ApprovedAt = &protoReturn.ApprovedAt
	}
	if protoReturn.CompletedAt != "" {
		res.CompletedAt = &protoReturn.CompletedAt
	}
	if protoReturn.CreatedAt != "" {
		res.CreatedAt = &protoReturn.CreatedAt
	}
	if protoReturn.UpdatedAt != "" {
		res.UpdatedAt = &protoReturn.UpdatedAt
	}
	if protoReturn.RejectionReason != "" {
		res.RejectionReason = &protoReturn.RejectionReason
	}

	if protoReturn.Verification != nil {
		var notes *string
		if protoReturn.Verification.Notes != "" {
			notes = &protoReturn.Verification.Notes
		}
		var createdAt *string
		if protoReturn.Verification.CreatedAt != "" {
			createdAt = &protoReturn.Verification.CreatedAt
		}
		var updatedAt *string
		if protoReturn.Verification.UpdatedAt != "" {
			updatedAt = &protoReturn.Verification.UpdatedAt
		}

		res.Verification = &model.ProductVerification{
			ID:          protoReturn.Verification.Id,
			ReturnID:    protoReturn.Verification.ReturnId,
			VerifiedBy:  protoReturn.Verification.VerifiedBy,
			Condition:   protoReturn.Verification.Condition,
			IsApproved:  protoReturn.Verification.IsApproved,
			ProofImages: protoReturn.Verification.ProofImages,
			Notes:       notes,
			VerifiedAt:  protoReturn.Verification.VerifiedAt,
			CreatedAt:   createdAt,
			UpdatedAt:   updatedAt,
		}
	}

	return res
}

func ToModelReturnOrder(protoReturn *returnpb.ReturnOrder) *model.ReturnOrder {
	return MapReturnOrder(protoReturn)
}

func MapReturnOrders(returns []*returnpb.ReturnOrder) []*model.ReturnOrder {
	if returns == nil {
		return []*model.ReturnOrder{}
	}
	res := make([]*model.ReturnOrder, 0, len(returns))
	for _, ret := range returns {
		if ret != nil {
			res = append(res, MapReturnOrder(ret))
		}
	}
	return res
}

func MapExchange(protoExc *returnpb.Exchange) *model.Exchange {
	if protoExc == nil {
		return nil
	}

	res := &model.Exchange{
		ID:                protoExc.Id,
		UserID:            protoExc.UserId,
		OldBrand:          protoExc.OldBrand,
		OldModel:          protoExc.OldModel,
		BaseExchangeValue: protoExc.BaseExchangeValue,
		EstimatedDiscount: protoExc.EstimatedDiscount,
		Status:            protoExc.Status,
	}

	if protoExc.ProductId != "" {
		res.ProductID = &protoExc.ProductId
	}
	if protoExc.ExchangeDeviceCatalogId != "" {
		res.ExchangeDeviceCatalogID = &protoExc.ExchangeDeviceCatalogId
	}

	if protoExc.OldVariant != "" {
		res.OldVariant = &protoExc.OldVariant
	}
	if protoExc.QuoteId != "" {
		res.QuoteID = &protoExc.QuoteId
	}
	if protoExc.SerialOrImei != "" {
		res.SerialOrImei = &protoExc.SerialOrImei
	}
	if protoExc.FinalDiscount != 0 || protoExc.Status != "PENDING_ESTIMATION" {
		fd := protoExc.FinalDiscount
		res.FinalDiscount = &fd
	}
	if protoExc.PriceDifference != 0 {
		pd := protoExc.PriceDifference
		res.PriceDifference = &pd
	}
	if protoExc.CreatedAt != "" {
		res.CreatedAt = &protoExc.CreatedAt
	}
	if protoExc.UpdatedAt != "" {
		res.UpdatedAt = &protoExc.UpdatedAt
	}

	if len(protoExc.Answers) > 0 {
		res.Answers = make([]*model.ExchangeQuestionAnswer, 0, len(protoExc.Answers))
		for _, a := range protoExc.Answers {
			if a == nil {
				continue
			}
			ansModel := &model.ExchangeQuestionAnswer{
				ID:              a.Id,
				ExchangeID:      a.ExchangeId,
				QuestionKey:     a.QuestionKey,
				AnswerValue:     a.AnswerValue,
				DeductionAmount: a.DeductionAmount,
			}
			if a.CreatedAt != "" {
				ansModel.CreatedAt = &a.CreatedAt
			}
			res.Answers = append(res.Answers, ansModel)
		}
	}

	if protoExc.Verification != nil {
		var notes *string
		if protoExc.Verification.Notes != "" {
			notes = &protoExc.Verification.Notes
		}
		var createdAt *string
		if protoExc.Verification.CreatedAt != "" {
			createdAt = &protoExc.Verification.CreatedAt
		}
		var updatedAt *string
		if protoExc.Verification.UpdatedAt != "" {
			updatedAt = &protoExc.Verification.UpdatedAt
		}

		res.Verification = &model.ExchangeVerification{
			ID:               protoExc.Verification.Id,
			ExchangeID:       protoExc.Verification.ExchangeId,
			VerifiedBy:       protoExc.Verification.VerifiedBy,
			ConditionRating:  protoExc.Verification.ConditionRating,
			IsApproved:       protoExc.Verification.IsApproved,
			ScreenIntact:     protoExc.Verification.ScreenIntact,
			BodyIntact:       protoExc.Verification.BodyIntact,
			Functional:       protoExc.Verification.Functional,
			SerialMatches:    protoExc.Verification.SerialMatches,
			AdjustedDiscount: protoExc.Verification.AdjustedDiscount,
			ProofImages:      protoExc.Verification.ProofImages,
			Notes:            notes,
			VerifiedAt:       protoExc.Verification.VerifiedAt,
			CreatedAt:        createdAt,
			UpdatedAt:        updatedAt,
		}
	}

	return res
}

func ToModelExchange(protoExc *returnpb.Exchange) *model.Exchange {
	return MapExchange(protoExc)
}

func MapExchangeQuote(protoQuote *returnpb.ExchangeQuote) *model.ExchangeQuote {
	if protoQuote == nil {
		return nil
	}

	answers := make([]*model.ExchangeQuoteAnswer, 0, len(protoQuote.Answers))
	for _, a := range protoQuote.Answers {
		answers = append(answers, &model.ExchangeQuoteAnswer{
			ID:              a.Id,
			QuoteID:         a.QuoteId,
			QuestionKey:     a.QuestionKey,
			OptionKey:       a.OptionKey,
			DeductionAmount: a.DeductionAmount,
			IsFatal:         a.IsFatal,
		})
	}

	var variant *string
	if protoQuote.OldVariant != "" {
		variant = &protoQuote.OldVariant
	}
	var serial *string
	if protoQuote.SerialOrImei != "" {
		serial = &protoQuote.SerialOrImei
	}
	var reason *string
	if protoQuote.DisqualificationReason != "" {
		reason = &protoQuote.DisqualificationReason
	}
	var createdAt *string
	if protoQuote.CreatedAt != "" {
		createdAt = &protoQuote.CreatedAt
	}
	var updatedAt *string
	if protoQuote.UpdatedAt != "" {
		updatedAt = &protoQuote.UpdatedAt
	}

	return &model.ExchangeQuote{
		ID:                     protoQuote.Id,
		UserID:                 protoQuote.UserId,
		ProductID:              protoQuote.ProductId,
		CategoryCode:           protoQuote.CategoryCode,
		OldBrand:               protoQuote.OldBrand,
		OldModel:               protoQuote.OldModel,
		OldVariant:             variant,
		SerialOrImei:           serial,
		BaseExchangeValue:      protoQuote.BaseExchangeValue,
		TotalDeductions:        protoQuote.TotalDeductions,
		FinalDiscount:          protoQuote.FinalDiscount,
		IsDisqualified:         protoQuote.IsDisqualified,
		DisqualificationReason: reason,
		Status:                 protoQuote.Status,
		ExpiresAt:              protoQuote.ExpiresAt,
		Answers:                answers,
		CreatedAt:              createdAt,
		UpdatedAt:              updatedAt,
	}
}

func ToModelExchangeQuote(protoQuote *returnpb.ExchangeQuote) *model.ExchangeQuote {
	return MapExchangeQuote(protoQuote)
}
