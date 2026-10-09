package grpc

import (
	"context"
	"log/slog"
	"time"

	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/service"
)

type PaymentGRPCServer struct {
	paymentpb.UnimplementedPaymentServiceServer
	paymentService service.PaymentService
}

func NewPaymentGRPCServer(paymentService service.PaymentService) *PaymentGRPCServer {
	return &PaymentGRPCServer{
		paymentService: paymentService,
	}
}

func (s *PaymentGRPCServer) ProcessPayment(ctx context.Context, req *paymentpb.ProcessPaymentRequest) (*paymentpb.ProcessPaymentResponse, error) {
	if req == nil {
		slog.WarnContext(ctx, "ProcessPayment received nil request")
		return nil, grpcclient.MapAppErrorToGRPC(appErrors.BadRequest("request cannot be nil"))
	}

	slog.InfoContext(ctx, "gRPC ProcessPayment called", "order_id", req.OrderId, "user_id", req.UserId, "idempotency_key", req.IdempotencyKey)

	dto := &service.ProcessPaymentDTO{
		OrderID:        req.OrderId,
		UserID:         req.UserId,
		PaymentMethod:  req.PaymentMethod,
		IdempotencyKey: req.IdempotencyKey,
		ClientAmount:   req.Amount,
		ClientCurrency: req.Currency,
	}

	payment, err := s.paymentService.ProcessPayment(ctx, dto)
	if err != nil {
		slog.WarnContext(ctx, "gRPC ProcessPayment failed", "order_id", req.OrderId, "error", err)
		return nil, grpcclient.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC ProcessPayment completed successfully", "payment_id", payment.ID, "status", payment.Status)
	return &paymentpb.ProcessPaymentResponse{
		Payment: mapModelToProtoPayment(payment),
	}, nil
}

func (s *PaymentGRPCServer) GetPayment(ctx context.Context, req *paymentpb.GetPaymentRequest) (*paymentpb.GetPaymentResponse, error) {
	if req == nil || req.Id == "" {
		slog.WarnContext(ctx, "GetPayment received invalid request")
		return nil, grpcclient.MapAppErrorToGRPC(appErrors.BadRequest("payment id is required"))
	}

	slog.InfoContext(ctx, "gRPC GetPayment called", "payment_id", req.Id)

	payment, err := s.paymentService.GetPaymentByID(ctx, req.Id)
	if err != nil {
		slog.WarnContext(ctx, "gRPC GetPayment failed", "payment_id", req.Id, "error", err)
		return nil, grpcclient.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC GetPayment retrieved successfully", "payment_id", payment.ID)
	return &paymentpb.GetPaymentResponse{
		Payment: mapModelToProtoPayment(payment),
	}, nil
}

func (s *PaymentGRPCServer) GetPaymentByOrderID(ctx context.Context, req *paymentpb.GetPaymentByOrderIDRequest) (*paymentpb.GetPaymentResponse, error) {
	if req == nil || req.OrderId == "" {
		slog.WarnContext(ctx, "GetPaymentByOrderID received invalid request")
		return nil, grpcclient.MapAppErrorToGRPC(appErrors.BadRequest("order_id is required"))
	}

	slog.InfoContext(ctx, "gRPC GetPaymentByOrderID called", "order_id", req.OrderId)

	payment, err := s.paymentService.GetPaymentByOrderID(ctx, req.OrderId)
	if err != nil {
		slog.WarnContext(ctx, "gRPC GetPaymentByOrderID failed", "order_id", req.OrderId, "error", err)
		return nil, grpcclient.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC GetPaymentByOrderID retrieved successfully", "order_id", req.OrderId, "payment_id", payment.ID)
	return &paymentpb.GetPaymentResponse{
		Payment: mapModelToProtoPayment(payment),
	}, nil
}

func (s *PaymentGRPCServer) CreateRefund(ctx context.Context, req *paymentpb.CreateRefundRequest) (*paymentpb.CreateRefundResponse, error) {
	if req == nil || req.PaymentId == "" {
		slog.WarnContext(ctx, "CreateRefund received invalid request")
		return nil, grpcclient.MapAppErrorToGRPC(appErrors.BadRequest("payment_id is required"))
	}

	slog.InfoContext(ctx, "gRPC CreateRefund called", "payment_id", req.PaymentId, "amount", req.Amount)

	dto := &service.CreateRefundDTO{
		PaymentID:      req.PaymentId,
		Amount:         req.Amount,
		Reason:         req.Reason,
		IdempotencyKey: req.IdempotencyKey,
	}

	refund, err := s.paymentService.CreateRefund(ctx, dto)
	if err != nil {
		slog.WarnContext(ctx, "gRPC CreateRefund failed", "payment_id", req.PaymentId, "error", err)
		return nil, grpcclient.MapAppErrorToGRPC(err)
	}

	slog.InfoContext(ctx, "gRPC CreateRefund completed successfully", "refund_id", refund.ID, "status", refund.Status)
	return &paymentpb.CreateRefundResponse{
		RefundId:        refund.ID,
		Status:          string(refund.Status),
		Amount:          refund.Amount,
		GatewayRefundId: refund.GatewayRefundID,
		CreatedAt:       refund.CreatedAt.Format(time.RFC3339),
	}, nil
}

func mapModelToProtoPayment(p *model.Payment) *paymentpb.Payment {
	if p == nil {
		return nil
	}

	return &paymentpb.Payment{
		Id:                   p.ID,
		OrderId:              p.OrderID,
		Amount:               p.Amount,
		Currency:             p.Currency,
		PaymentMethod:        p.PaymentMethod,
		Status:               string(p.Status),
		TransactionReference: p.TransactionID,
		GatewayTransactionId: p.GatewayTransactionID,
		CreatedAt:            p.CreatedAt.Format(time.RFC3339),
		UserId:               p.UserID,
		IdempotencyKey:       p.IdempotencyKey,
		FailureReason:        p.FailureReason,
		UpdatedAt:            p.UpdatedAt.Format(time.RFC3339),
	}
}
