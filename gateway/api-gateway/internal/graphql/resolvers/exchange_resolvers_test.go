package resolvers

import (
	"context"
	"testing"
	"time"

	returnpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/return"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/graphql/model"
	"github.com/marees-godev/GoCart-Server/gateway/api-gateway/internal/grpc"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	grpcPkg "google.golang.org/grpc"
)

type mockExchangeReturnClient struct {
	returnpb.ReturnServiceClient
}

func (m *mockExchangeReturnClient) GetExchangeQuestions(ctx context.Context, in *returnpb.GetExchangeQuestionsRequest, opts ...grpcPkg.CallOption) (*returnpb.GetExchangeQuestionsResponse, error) {
	return &returnpb.GetExchangeQuestionsResponse{
		Questions: []*returnpb.ExchangeDiagnosticQuestion{
			{
				Id:           "q-1",
				CategoryCode: in.CategoryCode,
				QuestionKey:  "POWER_ON",
				QuestionText: "Does device power on?",
				DisplayOrder: 1,
				Options: []*returnpb.ExchangeQuestionOption{
					{
						Id:          "opt-1",
						OptionKey:   "YES",
						OptionLabel: "Yes",
					},
					{
						Id:                  "opt-2",
						OptionKey:           "NO",
						OptionLabel:         "No",
						IsFatalDisqualifier: true,
						RejectionReason:     "Device must power on",
					},
				},
			},
		},
		Devices: []*returnpb.ExchangeDeviceOption{
			{
				Id:                "dev-1",
				CategoryCode:      "SMARTPHONE",
				Brand:             "Apple",
				Model:             "iPhone 13",
				Variant:           "128GB",
				BaseExchangeValue: 20000,
			},
		},
	}, nil
}

func (m *mockExchangeReturnClient) CalculateExchangeValuation(ctx context.Context, in *returnpb.CalculateExchangeValuationRequest, opts ...grpcPkg.CallOption) (*returnpb.CalculateExchangeValuationResponse, error) {
	for _, a := range in.Answers {
		if a.OptionKey == "NO" {
			return &returnpb.CalculateExchangeValuationResponse{
				IsEligible:      false,
				RejectionReason: "Device does not power on",
				FinalDiscount:   0,
			}, nil
		}
	}

	return &returnpb.CalculateExchangeValuationResponse{
		IsEligible:        true,
		BaseExchangeValue: 20000,
		TotalDeductions:   2000,
		FinalDiscount:     15000,
		Deductions: []*returnpb.ExchangeDeductionDetail{
			{
				QuestionKey:     "BODY_CONDITION",
				OptionKey:       "MINOR_SCRATCHES",
				DeductionAmount: 2000,
			},
		},
	}, nil
}

func (m *mockExchangeReturnClient) LockExchangeQuote(ctx context.Context, in *returnpb.LockExchangeQuoteRequest, opts ...grpcPkg.CallOption) (*returnpb.LockExchangeQuoteResponse, error) {
	return &returnpb.LockExchangeQuoteResponse{
		Quote: &returnpb.ExchangeQuote{
			Id:                "quote-123",
			UserId:            in.UserId,
			ProductId:         in.ProductId,
			CategoryCode:      in.CategoryCode,
			OldBrand:          in.OldBrand,
			OldModel:          in.OldModel,
			OldVariant:        in.OldVariant,
			SerialOrImei:      in.SerialOrImei,
			BaseExchangeValue: 20000,
			FinalDiscount:     15000,
			Status:            "LOCKED",
			ExpiresAt:         time.Now().Add(30 * time.Minute).Format(time.RFC3339),
			CreatedAt:         time.Now().Format(time.RFC3339),
		},
	}, nil
}

func (m *mockExchangeReturnClient) GetExchangeQuote(ctx context.Context, in *returnpb.GetExchangeQuoteRequest, opts ...grpcPkg.CallOption) (*returnpb.GetExchangeQuoteResponse, error) {
	return &returnpb.GetExchangeQuoteResponse{
		Quote: &returnpb.ExchangeQuote{
			Id:                in.Id,
			UserId:            "user-test-1",
			ProductId:         "prod-1",
			CategoryCode:      "SMARTPHONE",
			OldBrand:          "Apple",
			OldModel:          "iPhone 13",
			BaseExchangeValue: 20000,
			FinalDiscount:     15000,
			Status:            "LOCKED",
			ExpiresAt:         time.Now().Add(25 * time.Minute).Format(time.RFC3339),
		},
	}, nil
}

func (m *mockExchangeReturnClient) ApplyExchangeToOrder(ctx context.Context, in *returnpb.ApplyExchangeToOrderRequest, opts ...grpcPkg.CallOption) (*returnpb.ApplyExchangeToOrderResponse, error) {
	return &returnpb.ApplyExchangeToOrderResponse{
		Exchange: &returnpb.Exchange{
			Id:                "exc-999",
			QuoteId:           in.QuoteId,
			UserId:            in.UserId,
			Status:            "EXCHANGE_REQUESTED",
			EstimatedDiscount: 15000,
		},
	}, nil
}

func (m *mockExchangeReturnClient) ResolveExchangeMismatch(ctx context.Context, in *returnpb.ResolveExchangeMismatchRequest, opts ...grpcPkg.CallOption) (*returnpb.ResolveExchangeMismatchResponse, error) {
	status := "COMPLETED"
	if in.Action == "DECLINE_AND_CANCEL" {
		status = "CANCELLED"
	}
	return &returnpb.ResolveExchangeMismatchResponse{
		Exchange: &returnpb.Exchange{
			Id:     in.ExchangeId,
			Status: status,
		},
	}, nil
}

func TestExchangeResolvers(t *testing.T) {
	clients := &grpc.Clients{
		ReturnClient: &mockExchangeReturnClient{},
	}
	resolver := NewResolver(clients)

	ctx := auth.WithUser(context.Background(), &auth.UserContext{
		UserID: "user-test-1",
		Role:   "CUSTOMER",
	})

	t.Run("ExchangeQuestions", func(t *testing.T) {
		cat := "SMARTPHONE"
		resp, err := resolver.Query().ExchangeQuestions(ctx, &cat, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(resp.Questions) != 1 || resp.Questions[0].QuestionKey != "POWER_ON" {
			t.Fatalf("unexpected questions: %+v", resp.Questions)
		}
		if len(resp.Devices) != 1 || resp.Devices[0].Brand != "Apple" {
			t.Fatalf("unexpected devices: %+v", resp.Devices)
		}
	})

	t.Run("CalculateExchangeValuation - Eligible", func(t *testing.T) {
		cat := "SMARTPHONE"
		resp, err := resolver.Mutation().CalculateExchangeValuation(ctx, model.CalculateExchangeValuationInput{
			ProductID:    "prod-1",
			CategoryCode: &cat,
			OldBrand:     "Apple",
			OldModel:     "iPhone 13",
			Answers: []*model.ExchangeUserAnswerInput{
				{QuestionKey: "POWER_ON", OptionKey: "YES"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !resp.IsEligible || resp.FinalDiscount != 15000 {
			t.Fatalf("expected eligible valuation with 15000 discount, got: %+v", resp)
		}
	})

	t.Run("CalculateExchangeValuation - Disqualified", func(t *testing.T) {
		resp, err := resolver.Mutation().CalculateExchangeValuation(ctx, model.CalculateExchangeValuationInput{
			ProductID: "prod-1",
			OldBrand:  "Apple",
			OldModel:  "iPhone 13",
			Answers: []*model.ExchangeUserAnswerInput{
				{QuestionKey: "POWER_ON", OptionKey: "NO"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if resp.IsEligible || resp.RejectionReason == nil || *resp.RejectionReason != "Device does not power on" {
			t.Fatalf("expected disqualified response, got: %+v", resp)
		}
	})

	t.Run("LockExchangeQuote - Success", func(t *testing.T) {
		quote, err := resolver.Mutation().LockExchangeQuote(ctx, model.LockExchangeQuoteInput{
			ProductID: "prod-1",
			OldBrand:  "Apple",
			OldModel:  "iPhone 13",
			Answers: []*model.ExchangeUserAnswerInput{
				{QuestionKey: "POWER_ON", OptionKey: "YES"},
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quote.ID != "quote-123" || quote.FinalDiscount != 15000 || quote.Status != "LOCKED" {
			t.Fatalf("unexpected quote: %+v", quote)
		}
	})

	t.Run("LockExchangeQuote - Unauthenticated", func(t *testing.T) {
		_, err := resolver.Mutation().LockExchangeQuote(context.Background(), model.LockExchangeQuoteInput{
			ProductID: "prod-1",
			OldBrand:  "Apple",
			OldModel:  "iPhone 13",
		})
		if err == nil {
			t.Fatal("expected unauthorized error")
		}
	})

	t.Run("ExchangeQuote - Authorized and Forbidden checks", func(t *testing.T) {
		quote, err := resolver.Query().ExchangeQuote(ctx, "quote-123")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if quote.ID != "quote-123" {
			t.Fatalf("expected quote-123, got: %s", quote.ID)
		}

		// Other user forbidden
		otherCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "other-user",
			Role:   "CUSTOMER",
		})
		_, err = resolver.Query().ExchangeQuote(otherCtx, "quote-123")
		if err == nil {
			t.Fatal("expected forbidden error for different user")
		}

		// Admin permitted
		adminCtx := auth.WithUser(context.Background(), &auth.UserContext{
			UserID: "admin-user",
			Role:   "ADMIN",
		})
		adminQuote, err := resolver.Query().ExchangeQuote(adminCtx, "quote-123")
		if err != nil {
			t.Fatalf("admin should be allowed to view quote: %v", err)
		}
		if adminQuote == nil {
			t.Fatal("expected quote for admin")
		}
	})

	t.Run("ApplyExchangeToOrder - Success", func(t *testing.T) {
		exc, err := resolver.Mutation().ApplyExchangeToOrder(ctx, model.ApplyExchangeToOrderInput{
			QuoteID:     "quote-123",
			OrderID:     "order-456",
			OrderItemID: "item-789",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exc.ID != "exc-999" || exc.Status != "EXCHANGE_REQUESTED" || exc.QuoteID == nil || *exc.QuoteID != "quote-123" {
			t.Fatalf("unexpected exchange: %+v", exc)
		}
	})

	t.Run("ResolveExchangeMismatch - Accept and Decline", func(t *testing.T) {
		exc, err := resolver.Mutation().ResolveExchangeMismatch(ctx, model.ResolveExchangeMismatchInput{
			ExchangeID: "exc-999",
			Action:     "ACCEPT_REVISED",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if exc.Status != "COMPLETED" {
			t.Fatalf("expected COMPLETED, got: %s", exc.Status)
		}

		excCancel, err := resolver.Mutation().ResolveExchangeMismatch(ctx, model.ResolveExchangeMismatchInput{
			ExchangeID: "exc-999",
			Action:     "DECLINE_AND_CANCEL",
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if excCancel.Status != "CANCELLED" {
			t.Fatalf("expected CANCELLED, got: %s", excCancel.Status)
		}
	})
}