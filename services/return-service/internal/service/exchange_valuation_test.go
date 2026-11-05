package service_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	productpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/product"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/service"
)

type mockProductClientForExchange struct {
	prod *productpb.Product
	err  error
}

func (m *mockProductClientForExchange) GetProduct(ctx context.Context, id string) (*productpb.Product, error) {
	return m.prod, m.err
}

func TestExchangeService_GetExchangeQuestions_Success(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p1", IsExchangeEligible: true},
	}, 7)

	questions, devices, err := svc.GetExchangeQuestions(ctx, &dto.GetExchangeQuestionsDTO{
		CategoryCode: "SMARTPHONE",
		ProductID:    "p1",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(questions) == 0 {
		t.Fatal("expected non-empty questions")
	}
	if len(devices) == 0 {
		t.Fatal("expected non-empty devices")
	}
	if questions[0].QuestionKey != "powers_on" {
		t.Fatalf("expected question key 'powers_on', got %s", questions[0].QuestionKey)
	}
	if devices[0].Brand != "Apple" {
		t.Fatalf("expected brand 'Apple', got %s", devices[0].Brand)
	}
}

func TestExchangeService_GetExchangeQuestions_IneligibleProduct(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p2", IsExchangeEligible: false},
	}, 7)

	_, _, err := svc.GetExchangeQuestions(ctx, &dto.GetExchangeQuestionsDTO{
		CategoryCode: "SMARTPHONE",
		ProductID:    "p2",
	})
	if !errors.Is(err, service.ErrProductNotExchangeEligible) {
		t.Fatalf("expected ErrProductNotExchangeEligible, got: %v", err)
	}
}

func TestExchangeService_CalculateValuation_FatalDisqualifier(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p1", IsExchangeEligible: true},
	}, 7)

	// Answering "no" to powers_on is a fatal disqualifier
	res, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
		ProductID:    "p1",
		CategoryCode: "SMARTPHONE",
		OldBrand:     "Apple",
		OldModel:     "iPhone 13",
		Answers: []*dto.ExchangeUserAnswerDTO{
			{QuestionKey: "powers_on", OptionKey: "no"},
			{QuestionKey: "screen_cracks", OptionKey: "none"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.IsEligible {
		t.Fatal("expected device to be ineligible due to fatal condition")
	}
	if res.FinalDiscount != 0 {
		t.Fatalf("expected 0 discount, got: %f", res.FinalDiscount)
	}
	if !strings.Contains(res.RejectionReason, "Device does not power on") {
		t.Fatalf("expected rejection reason to mention 'Device does not power on', got: %s", res.RejectionReason)
	}
}

func TestExchangeService_CalculateValuation_ConditionPenalties(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p1", IsExchangeEligible: true, MaxExchangeDiscount: 30000},
	}, 7)

	// Powers on (0 deduction), screen cracked (4000 deduction) -> 26000 - 4000 = 22000
	res, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
		ProductID:    "p1",
		CategoryCode: "SMARTPHONE",
		OldBrand:     "Apple",
		OldModel:     "iPhone 13",
		Answers: []*dto.ExchangeUserAnswerDTO{
			{QuestionKey: "powers_on", OptionKey: "yes"},
			{QuestionKey: "screen_cracks", OptionKey: "cracked"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !res.IsEligible {
		t.Fatal("expected device to be eligible")
	}
	if res.BaseExchangeValue != 26000 {
		t.Fatalf("expected base value 26000, got: %f", res.BaseExchangeValue)
	}
	if res.TotalDeductions != 4000 {
		t.Fatalf("expected total deductions 4000, got: %f", res.TotalDeductions)
	}
	if res.FinalDiscount != 22000 {
		t.Fatalf("expected final discount 22000, got: %f", res.FinalDiscount)
	}
}

func TestExchangeService_LockQuote_And_ApplyToOrder(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	orderClient := &mockOrderClient{
		order: &orderpb.Order{Id: "ord-1", UserId: "user-123"},
	}
	svc := service.NewReturnService(repo, orderClient, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "prod-new", IsExchangeEligible: true},
	}, 7)

	// 1. Lock Exchange Quote
	quote, err := svc.LockExchangeQuote(ctx, &dto.LockExchangeQuoteDTO{
		UserID:       "user-123",
		ProductID:    "prod-new",
		CategoryCode: "SMARTPHONE",
		OldBrand:     "Apple",
		OldModel:     "iPhone 13",
		SerialOrIMEI: "354921098492019",
		Answers: []*dto.ExchangeUserAnswerDTO{
			{QuestionKey: "powers_on", OptionKey: "yes"},
			{QuestionKey: "screen_cracks", OptionKey: "cracked"},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if quote.ID == "" {
		t.Fatal("expected non-empty quote ID")
	}
	if quote.Status != model.ExchangeQuoteStatusLocked {
		t.Fatalf("expected status LOCKED, got %s", quote.Status)
	}
	if quote.FinalDiscount != 22000 {
		t.Fatalf("expected discount 22000, got %f", quote.FinalDiscount)
	}
	if !quote.ExpiresAt.After(time.Now().UTC()) {
		t.Fatal("expected quote expiry to be in the future")
	}

	// 2. Apply Exchange Quote to Order at checkout
	exc, err := svc.ApplyExchangeToOrder(ctx, &dto.ApplyExchangeToOrderDTO{
		QuoteID:     quote.ID,
		OrderID:     "ord-1",
		OrderItemID: "item-1",
		UserID:      "user-123",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exc.ID == "" {
		t.Fatal("expected non-empty exchange ID")
	}
	if exc.Status != model.ExchangeStatusRequested {
		t.Fatalf("expected status EXCHANGE_REQUESTED, got %s", exc.Status)
	}
	if exc.FinalDiscount != 22000 {
		t.Fatalf("expected discount 22000, got %f", exc.FinalDiscount)
	}
	if exc.SerialOrIMEI != "354921098492019" {
		t.Fatalf("expected serial '354921098492019', got %s", exc.SerialOrIMEI)
	}
	if len(repo.outboxEvents) == 0 {
		t.Fatal("expected outbox event to be stored")
	}

	// 3. Second attempt with the same quote should fail (already APPLIED / race prevention)
	_, err = svc.ApplyExchangeToOrder(ctx, &dto.ApplyExchangeToOrderDTO{
		QuoteID:     quote.ID,
		OrderID:     "ord-2",
		OrderItemID: "item-2",
		UserID:      "user-123",
	})
	if !errors.Is(err, service.ErrExchangeQuoteNotApplicable) {
		t.Fatalf("expected ErrExchangeQuoteNotApplicable on re-applying quote, got: %v", err)
	}
}

func TestExchangeService_ResolveExchangeMismatch(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{}, 7)

	now := time.Now().UTC()
	exc := &model.Exchange{
		ID:                "exc-reval",
		UserID:            "user-123",
		ProductID:         "prod-1",
		OldBrand:          "Apple",
		OldModel:          "iPhone 13",
		BaseExchangeValue: 26000.0,
		EstimatedDiscount: 22000.0,
		FinalDiscount:     18000.0, // Degraded condition at doorstep (-4000)
		PriceDifference:   4000.0,
		Status:            model.ExchangeStatusRevaluated,
		CreatedAt:         now,
		UpdatedAt:         now,
	}
	repo.exchanges[exc.ID] = exc

	// Case 1: Customer accepts revised discount and pays difference
	accepted, err := svc.ResolveExchangeMismatch(ctx, &dto.ResolveExchangeMismatchDTO{
		ExchangeID:    "exc-reval",
		Action:        "ACCEPT_REVISED",
		PaymentMethod: "UPI_DOORSTEP",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if accepted.Status != model.ExchangeStatusCompleted {
		t.Fatalf("expected status COMPLETED, got %s", accepted.Status)
	}
	if accepted.FinalDiscount != 18000 {
		t.Fatalf("expected final discount 18000, got %f", accepted.FinalDiscount)
	}
	if accepted.PriceDifference != 4000 {
		t.Fatalf("expected price difference 4000, got %f", accepted.PriceDifference)
	}
	if len(repo.outboxEvents) != 1 {
		t.Fatalf("expected 1 outbox event for ACCEPT_REVISED, got %d", len(repo.outboxEvents))
	}
	if repo.outboxEvents[0].EventType != events.EventTypeExchangeCompleted {
		t.Fatalf("expected event type %s, got %s", events.EventTypeExchangeCompleted, repo.outboxEvents[0].EventType)
	}

	// Case 2: Customer declines revised discount -> exchange cancelled
	exc.Status = model.ExchangeStatusRevaluated
	declined, err := svc.ResolveExchangeMismatch(ctx, &dto.ResolveExchangeMismatchDTO{
		ExchangeID: "exc-reval",
		Action:     "DECLINE_AND_CANCEL",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if declined.Status != model.ExchangeStatusCancelled {
		t.Fatalf("expected status CANCELLED, got %s", declined.Status)
	}
	if declined.FinalDiscount != 0 {
		t.Fatalf("expected final discount 0, got %f", declined.FinalDiscount)
	}
	if declined.PriceDifference != 22000 {
		t.Fatalf("expected price difference 22000, got %f", declined.PriceDifference)
	}
	if len(repo.outboxEvents) != 2 {
		t.Fatalf("expected 2 outbox events after DECLINE_AND_CANCEL, got %d", len(repo.outboxEvents))
	}
}

func TestExchangeService_CalculateValuation_ValidationErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p1", IsExchangeEligible: true},
	}, 7)

	t.Run("Empty answers rejected", func(t *testing.T) {
		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers:      []*dto.ExchangeUserAnswerDTO{},
		})
		if !errors.Is(err, service.ErrMissingRequiredQuestion) {
			t.Fatalf("expected ErrMissingRequiredQuestion, got: %v", err)
		}
	})

	t.Run("Partial answers missing required question rejected", func(t *testing.T) {
		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers: []*dto.ExchangeUserAnswerDTO{
				{QuestionKey: "powers_on", OptionKey: "yes"},
			},
		})
		if !errors.Is(err, service.ErrMissingRequiredQuestion) {
			t.Fatalf("expected ErrMissingRequiredQuestion, got: %v", err)
		}
	})

	t.Run("Unknown question rejected", func(t *testing.T) {
		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers: []*dto.ExchangeUserAnswerDTO{
				{QuestionKey: "battery_health", OptionKey: "good"},
				{QuestionKey: "powers_on", OptionKey: "yes"},
				{QuestionKey: "screen_cracks", OptionKey: "none"},
			},
		})
		if !errors.Is(err, service.ErrUnknownExchangeQuestion) {
			t.Fatalf("expected ErrUnknownExchangeQuestion, got: %v", err)
		}
	})

	t.Run("Unknown option rejected", func(t *testing.T) {
		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers: []*dto.ExchangeUserAnswerDTO{
				{QuestionKey: "powers_on", OptionKey: "maybe"},
				{QuestionKey: "screen_cracks", OptionKey: "none"},
			},
		})
		if !errors.Is(err, service.ErrUnknownExchangeOption) {
			t.Fatalf("expected ErrUnknownExchangeOption, got: %v", err)
		}
	})

	t.Run("Duplicate answer rejected", func(t *testing.T) {
		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers: []*dto.ExchangeUserAnswerDTO{
				{QuestionKey: "powers_on", OptionKey: "yes"},
				{QuestionKey: "powers_on", OptionKey: "no"},
				{QuestionKey: "screen_cracks", OptionKey: "none"},
			},
		})
		if !errors.Is(err, service.ErrDuplicateExchangeAnswer) {
			t.Fatalf("expected ErrDuplicateExchangeAnswer, got: %v", err)
		}
	})
}

func TestExchangeService_LockQuote_Disqualified_Rejection(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()
	svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
		prod: &productpb.Product{Id: "p1", IsExchangeEligible: true},
	}, 7)

	_, err := svc.LockExchangeQuote(ctx, &dto.LockExchangeQuoteDTO{
		UserID:       "user-123",
		ProductID:    "p1",
		CategoryCode: "SMARTPHONE",
		OldBrand:     "Apple",
		OldModel:     "iPhone 13",
		Answers: []*dto.ExchangeUserAnswerDTO{
			{QuestionKey: "powers_on", OptionKey: "no"},
			{QuestionKey: "screen_cracks", OptionKey: "none"},
		},
	})
	if !errors.Is(err, service.ErrExchangeDisqualified) {
		t.Fatalf("expected ErrExchangeDisqualified when locking quote for ineligible device, got: %v", err)
	}
}

func TestExchangeService_CalculateValuation_ProductServiceErrors(t *testing.T) {
	ctx := context.Background()
	repo := newMockReturnRepo()

	validAnswers := []*dto.ExchangeUserAnswerDTO{
		{QuestionKey: "powers_on", OptionKey: "yes"},
		{QuestionKey: "screen_cracks", OptionKey: "none"},
	}

	t.Run("Propagate product service lookup error", func(t *testing.T) {
		expectedErr := errors.New("product service rpc failed")
		svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
			err: expectedErr,
		}, 7)

		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers:      validAnswers,
		})
		if !errors.Is(err, expectedErr) {
			t.Fatalf("expected error %v, got: %v", expectedErr, err)
		}
	})

	t.Run("Product client is nil returns ErrProductServiceUnavailable", func(t *testing.T) {
		svc := service.NewReturnService(repo, &mockOrderClient{}, nil, 7)

		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers:      validAnswers,
		})
		if !errors.Is(err, service.ErrProductServiceUnavailable) {
			t.Fatalf("expected ErrProductServiceUnavailable, got: %v", err)
		}
	})

	t.Run("Product not found returns ErrProductNotFound", func(t *testing.T) {
		svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
			prod: nil,
		}, 7)

		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers:      validAnswers,
		})
		if !errors.Is(err, service.ErrProductNotFound) {
			t.Fatalf("expected ErrProductNotFound, got: %v", err)
		}
	})

	t.Run("Ineligible product returns ErrProductNotExchangeEligible", func(t *testing.T) {
		svc := service.NewReturnService(repo, &mockOrderClient{}, &mockProductClientForExchange{
			prod: &productpb.Product{Id: "p1", IsExchangeEligible: false},
		}, 7)

		_, err := svc.CalculateExchangeValuation(ctx, &dto.CalculateExchangeValuationDTO{
			ProductID:    "p1",
			CategoryCode: "SMARTPHONE",
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers:      validAnswers,
		})
		if !errors.Is(err, service.ErrProductNotExchangeEligible) {
			t.Fatalf("expected ErrProductNotExchangeEligible, got: %v", err)
		}
	})
}