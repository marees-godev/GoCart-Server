package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	paymentpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/payment"
	"github.com/marees-godev/GoCart-Server/pkg/auth"
	"github.com/marees-godev/GoCart-Server/pkg/grpcclient"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/client"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/events"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/return-service/internal/repository"
)

var (
	ErrReturnReasonRequired           = errors.New("return reason is mandatory")
	ErrReturnIDRequired               = errors.New("return id is required")
	ErrReviewerIDRequired             = errors.New("reviewer id is required")
	ErrOrderItemRequired              = errors.New("order item id is required")
	ErrOrderIDRequired                = errors.New("order id is required")
	ErrUserIDRequired                 = errors.New("user id is required")
	ErrDuplicateReturnRequest         = errors.New("duplicate return request for order item")
	ErrOrderNotDelivered              = errors.New("order must be delivered to request a return")
	ErrOrderItemNotFound              = errors.New("order item not found in order")
	ErrUnauthorizedReturnRequest      = errors.New("order does not belong to authenticated user")
	ErrReturnWindowExpired            = errors.New("return window has expired")
	ErrProductNotReturnable           = errors.New("product is not returnable")
	ErrInvalidReturnStatus            = errors.New("invalid return status")
	ErrOrderNotFound                  = errors.New("order not found")
	ErrInvalidReturnType              = errors.New("invalid return type")
	ErrProofImagesRequired            = errors.New("at least one proof image is required for product verification")
	ErrVerifiedByRequired             = errors.New("verified by delivery partner ID is required")
	ErrInvalidVerificationCondition   = errors.New("invalid product verification condition")
	ErrInvalidExchangeConditionRating = errors.New("invalid exchange condition rating")
	ErrReturnNotFound                 = errors.New("return not found")
	ErrReturnNotVerifiable            = errors.New("return cannot be verified in current status")
	ErrMultipleItemsNotSupported      = errors.New("only one item per return request is supported")
	ErrOrderItemMismatch              = errors.New("order item id and item ids mismatch")
	ErrInvalidStateTransition         = errors.New("invalid return status transition")
	ErrCustomerCannotReviewOwnReturn  = errors.New("customer cannot approve or reject their own return")
	ErrRejectionReasonRequired        = errors.New("rejection reason is required")
	ErrExchangeNotFound               = errors.New("exchange record not found")
	ErrExchangeNotVerifiable          = errors.New("exchange cannot be verified in current status")
	ErrExchangeBrandModelRequired     = errors.New("old brand and old model are required for exchange")
	ErrExchangeNewProductRequired     = errors.New("new product id is required for exchange")
	ErrExchangeFunctionalRequired     = errors.New("device must be functional and serial must match to approve exchange")
	ErrReturnNotEligibleForRefund     = errors.New("refund can only be triggered after return is received or verified")
	ErrRefundExceedsPaidAmount        = errors.New("refund amount cannot exceed total paid amount")
	ErrPaymentNotFound                = errors.New("payment not found for order")
	ErrRefundFailed                   = errors.New("payment refund failed")
)

type ReturnService interface {
	RequestReturn(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error)
	GetReturn(ctx context.Context, id string) (*model.Return, error)
	UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus, rejectionReason ...string) (*model.Return, error)
	ApproveReturn(ctx context.Context, id string, reviewerID string) (*model.Return, error)
	RejectReturn(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error)
	VerifyProduct(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error)
	ListReturns(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error)
	ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error)

	CreateExchange(ctx context.Context, req *dto.CreateExchangeDTO) (*model.Exchange, error)
	GetExchange(ctx context.Context, id string) (*model.Exchange, error)
	VerifyExchangeProduct(ctx context.Context, req *dto.VerifyExchangeProductDTO) (*model.Exchange, *model.ExchangeVerification, error)
	TriggerRefund(ctx context.Context, returnID string) (*model.Return, error)
	ProcessRefund(ctx context.Context, returnID string) (*model.Return, error)
}

type returnService struct {
	repo             repository.ReturnRepository
	orderClient      client.OrderClient
	productClient    client.ProductClient
	paymentClient    client.PaymentClient
	returnWindowDays int
	logger           *slog.Logger
}

func NewReturnService(
	repo repository.ReturnRepository,
	orderClient client.OrderClient,
	productClient client.ProductClient,
	returnWindowDays int,
	opts ...any,
) ReturnService {
	if returnWindowDays <= 0 {
		returnWindowDays = 7
	}
	var l *slog.Logger
	var payClient client.PaymentClient
	for _, opt := range opts {
		if opt == nil {
			continue
		}
		switch v := opt.(type) {
		case *slog.Logger:
			l = v
		case client.PaymentClient:
			payClient = v
		}
	}
	if l == nil {
		l = slog.Default()
	}
	return &returnService{
		repo:             repo,
		orderClient:      orderClient,
		productClient:    productClient,
		paymentClient:    payClient,
		returnWindowDays: returnWindowDays,
		logger:           l,
	}
}

func (s *returnService) SetPaymentClient(c client.PaymentClient) {
	s.paymentClient = c
}

func (s *returnService) RequestReturn(ctx context.Context, req *dto.RequestReturnDTO) (*model.Return, error) {
	if req == nil {
		s.logger.WarnContext(ctx, "return request rejected: nil request payload")
		return nil, errors.New("request cannot be nil")
	}

	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		s.logger.WarnContext(ctx, "return request rejected: user_id is required")
		return nil, ErrUserIDRequired
	}

	req.OrderID = strings.TrimSpace(req.OrderID)
	if req.OrderID == "" {
		s.logger.WarnContext(ctx, "return request rejected: order_id is required", slog.String("user_id", req.UserID))
		return nil, ErrOrderIDRequired
	}

	orderItemID := strings.TrimSpace(req.OrderItemID)
	itemID := strings.TrimSpace(req.ItemID)
	if orderItemID != "" && itemID != "" && orderItemID != itemID {
		return nil, errors.New("order_item_id and item_id must match")
	}
	if orderItemID == "" {
		orderItemID = itemID
	}
	if orderItemID == "" {
		s.logger.WarnContext(ctx, "return request rejected: order_item_id is required",
			slog.String("order_id", req.OrderID),
			slog.String("user_id", req.UserID),
		)
		return nil, ErrOrderItemRequired
	}

	reason := strings.TrimSpace(req.Reason)
	if reason == "" {
		s.logger.WarnContext(ctx, "return request rejected: return reason is mandatory",
			slog.String("order_id", req.OrderID),
			slog.String("order_item_id", orderItemID),
		)
		return nil, ErrReturnReasonRequired
	}

	reqType := model.ReturnType(strings.ToUpper(strings.TrimSpace(req.Type)))
	if reqType == "" {
		reqType = model.ReturnTypeReturn
	}
	if !reqType.IsValid() {
		return nil, ErrInvalidReturnType
	}

	s.logger.InfoContext(ctx, "processing return request",
		slog.String("order_id", req.OrderID),
		slog.String("order_item_id", orderItemID),
		slog.String("user_id", req.UserID),
		slog.String("type", string(reqType)),
	)

	// 1. Fetch order from OrderService to validate status, user ownership, and item existence
	if s.orderClient == nil {
		s.logger.ErrorContext(ctx, "order client not configured")
		return nil, errors.New("order client not configured")
	}

	order, err := s.orderClient.GetOrder(ctx, req.OrderID, req.UserID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to retrieve order from order service",
			slog.String("order_id", req.OrderID),
			slog.String("user_id", req.UserID),
			slog.Any("error", err),
		)
		return nil, err
	}
	if order == nil {
		s.logger.WarnContext(ctx, "order not found for return request", slog.String("order_id", req.OrderID))
		return nil, ErrOrderNotFound
	}

	// 2. User association validation
	if order.GetUserId() != "" && order.GetUserId() != req.UserID {
		s.logger.WarnContext(ctx, "return request rejected: unauthorized user",
			slog.String("order_id", req.OrderID),
			slog.String("request_user_id", req.UserID),
			slog.String("order_user_id", order.GetUserId()),
		)
		return nil, ErrUnauthorizedReturnRequest
	}

	// 3. Order status must be DELIVERED
	orderStatus := strings.ToUpper(strings.TrimSpace(order.GetStatus()))
	if orderStatus == "" {
		orderStatus = strings.ToUpper(strings.TrimSpace(order.GetOrderStatus()))
	}
	if orderStatus != "DELIVERED" {
		s.logger.WarnContext(ctx, "return request rejected: order not delivered",
			slog.String("order_id", req.OrderID),
			slog.String("order_status", orderStatus),
		)
		return nil, ErrOrderNotDelivered
	}

	// 4. Verify the order item exists in the delivered order
	var targetItem *orderpb.OrderItem
	for _, it := range order.GetItems() {
		if it.GetId() == orderItemID || it.GetProductId() == orderItemID {
			targetItem = it
			break
		}
	}

	if targetItem == nil {
		var available []string
		for _, it := range order.GetItems() {
			available = append(available, fmt.Sprintf("[item_id=%s, product_id=%s, name=%s]", it.GetId(), it.GetProductId(), it.GetProductName()))
		}
		s.logger.WarnContext(ctx, "return request rejected: order item not found in order",
			slog.String("order_id", req.OrderID),
			slog.String("order_item_id", orderItemID),
			slog.Any("available_items", available),
		)
		return nil, ErrOrderItemNotFound
	}

	// Update orderItemID to canonical order item id
	if targetItem.GetId() != "" {
		orderItemID = targetItem.GetId()
	}

	// 5. Check for duplicate return request on this order item
	existing, err := s.repo.GetReturnByOrderItemID(ctx, orderItemID)
	if err != nil && !errors.Is(err, repository.ErrReturnNotFound) {
		s.logger.ErrorContext(ctx, "failed to check existing return request",
			slog.String("order_item_id", orderItemID),
			slog.Any("error", err),
		)
		return nil, err
	}
	if existing != nil {
		s.logger.WarnContext(ctx, "duplicate return request rejected",
			slog.String("order_item_id", orderItemID),
			slog.String("existing_return_id", existing.ID),
		)
		return nil, ErrDuplicateReturnRequest
	}

	// 6. Return window validation: 7 days of delivery ----> Todo will work on actual time for return window
	deliveryTime := parseDeliveryTime(order)
	returnWindow := time.Duration(s.returnWindowDays) * 24 * time.Hour
	if time.Since(deliveryTime) > returnWindow {
		s.logger.WarnContext(ctx, "return request rejected: return window expired",
			slog.String("order_id", req.OrderID),
			slog.Time("delivery_time", deliveryTime),
			slog.Int("return_window_days", s.returnWindowDays),
		)
		return nil, ErrReturnWindowExpired
	}

	// 7. Product returnability check
	if s.productClient != nil {
		product, err := s.productClient.GetProduct(ctx, targetItem.GetProductId())
		if err != nil {
			s.logger.WarnContext(ctx, "failed to fetch product for return eligibility check",
				slog.String("product_id", targetItem.GetProductId()),
				slog.Any("error", err),
			)
		} else if product != nil && !product.GetIsReturnable() {
			s.logger.WarnContext(ctx, "return request rejected: product is not returnable",
				slog.String("product_id", targetItem.GetProductId()),
				slog.String("order_item_id", orderItemID),
			)
			return nil, ErrProductNotReturnable
		}
	}

	// 8. Calculate refund amount
	refundAmount := targetItem.GetTotalPrice()
	if refundAmount <= 0 {
		refundAmount = targetItem.GetUnitPrice() * float64(targetItem.GetQuantity())
	}

	// 9. Persist return and outbox event
	now := time.Now().UTC()
	initialStatus := model.ReturnStatusRequested
	if reqType == model.ReturnTypeReplacement {
		initialStatus = model.ReturnStatusReplacementRequested
	} else if reqType == model.ReturnTypeExchange {
		initialStatus = model.ReturnStatusExchangeRequested
	}

	ret := &model.Return{
		ID:           uuid.New().String(),
		OrderID:      req.OrderID,
		OrderItemID:  orderItemID,
		UserID:       req.UserID,
		Reason:       reason,
		Status:       initialStatus,
		Type:         reqType,
		RefundAmount: refundAmount,
		RequestedAt:  now,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	outboxEvt, err := events.BuildReturnRequestedOutboxEvent(ret, targetItem.GetProductId())
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build return requested outbox event",
			slog.String("return_id", ret.ID),
			slog.Any("error", err),
		)
		return nil, err
	}

	if err := s.repo.CreateReturnWithOutbox(ctx, ret, outboxEvt); err != nil {
		s.logger.ErrorContext(ctx, "failed to persist return with outbox event",
			slog.String("return_id", ret.ID),
			slog.Any("error", err),
		)
		return nil, err
	}

	s.logger.InfoContext(ctx, "return request created successfully",
		slog.String("return_id", ret.ID),
		slog.String("order_id", ret.OrderID),
		slog.String("order_item_id", ret.OrderItemID),
		slog.String("user_id", ret.UserID),
		slog.String("type", string(ret.Type)),
		slog.Float64("refund_amount", ret.RefundAmount),
	)

	return ret, nil
}

func (s *returnService) GetReturn(ctx context.Context, id string) (*model.Return, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("return id is required")
	}
	s.logger.DebugContext(ctx, "retrieving return by id", slog.String("return_id", id))
	ret, err := s.repo.GetReturnByID(ctx, id)
	if err != nil && !errors.Is(err, repository.ErrReturnNotFound) {
		s.logger.ErrorContext(ctx, "failed to retrieve return by id",
			slog.String("return_id", id),
			slog.Any("error", err),
		)
	}
	return ret, err
}

func (s *returnService) UpdateReturnStatus(ctx context.Context, id string, status model.ReturnStatus, rejectionReason ...string) (*model.Return, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrReturnIDRequired
	}
	if !status.IsValid() {
		s.logger.WarnContext(ctx, "invalid return status update attempted",
			slog.String("return_id", id),
			slog.String("status", string(status)),
		)
		return nil, ErrInvalidReturnStatus
	}

	current, err := s.repo.GetReturnByID(ctx, id)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to retrieve return for status update",
			slog.String("return_id", id),
			slog.String("target_status", string(status)),
			slog.Any("error", err),
		)
		return nil, err
	}
	if current == nil {
		return nil, ErrReturnNotFound
	}

	if !current.Status.CanTransitionTo(status) {
		s.logger.WarnContext(ctx, "invalid return status transition rejected",
			slog.String("return_id", id),
			slog.String("current_status", string(current.Status)),
			slog.String("target_status", string(status)),
		)
		return nil, fmt.Errorf("%w: cannot transition return from %s to %s", ErrInvalidStateTransition, current.Status, status)
	}

	reviewerID := grpcclient.GetUserID(ctx)
	if reviewerID == "" {
		if u, ok := auth.UserFromContext(ctx); ok && u != nil {
			reviewerID = u.UserID
		}
	}
	if reviewerID == "" && len(rejectionReason) > 1 {
		reviewerID = strings.TrimSpace(rejectionReason[1])
	}

	// Customer cannot approve or reject their own return
	if status == model.ReturnStatusApproved || status == model.ReturnStatusRejected {
		if reviewerID != "" && reviewerID == current.UserID {
			s.logger.WarnContext(ctx, "customer cannot approve or reject their own return",
				slog.String("return_id", id),
				slog.String("user_id", current.UserID),
				slog.String("reviewer_id", reviewerID),
			)
			return nil, ErrCustomerCannotReviewOwnReturn
		}
	}

	var rejReason string
	if len(rejectionReason) > 0 {
		rejReason = strings.TrimSpace(rejectionReason[0])
	}
	if status == model.ReturnStatusRejected && rejReason == "" {
		return nil, ErrRejectionReasonRequired
	}

	if (status == model.ReturnStatusCompleted || status == model.ReturnStatusRefundInitiated) && current.Type == model.ReturnTypeReturn && s.paymentClient != nil {
		return s.TriggerRefund(ctx, id)
	}

	var outboxEvents []*outbox.Event

	// Publish ReturnApproved event
	if status == model.ReturnStatusApproved {
		now := time.Now().UTC()
		if current.ApprovedAt == nil {
			current.ApprovedAt = &now
		}
		current.UpdatedAt = now
		apprEvt, err := events.BuildReturnApprovedOutboxEvent(current, reviewerID)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to build return approved outbox event", slog.Any("error", err))
			return nil, err
		}
		outboxEvents = append(outboxEvents, apprEvt)
	}

	// Publish ReturnRejected event
	if status == model.ReturnStatusRejected {
		rejEvt, err := events.BuildReturnRejectedOutboxEvent(current, reviewerID, rejReason)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to build return rejected outbox event", slog.Any("error", err))
			return nil, err
		}
		outboxEvents = append(outboxEvents, rejEvt)
	}

	// Publish ReturnCompleted event
	if status == model.ReturnStatusCompleted {
		complEvt, err := events.BuildReturnCompletedOutboxEvent(current)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to build return completed outbox event", slog.Any("error", err))
			return nil, err
		}
		outboxEvents = append(outboxEvents, complEvt)
	}

	// Publish RefundInitiated event
	if status == model.ReturnStatusRefundInitiated {
		refInitEvt, err := events.BuildRefundInitiatedOutboxEvent(current, "", current.RefundAmount, "")
		if err == nil && refInitEvt != nil {
			outboxEvents = append(outboxEvents, refInitEvt)
		}
	}

	// Publish ReturnStatusUpdated event
	statusEvt, err := events.BuildReturnStatusUpdatedOutboxEvent(current, current.Status, status)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build return status updated outbox event", slog.Any("error", err))
		return nil, err
	}
	outboxEvents = append(outboxEvents, statusEvt)

	ret, err := s.repo.UpdateReturnStatusWithOutbox(ctx, id, status, rejReason, outboxEvents...)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to update return status",
			slog.String("return_id", id),
			slog.String("status", string(status)),
			slog.Any("error", err),
		)
		return nil, err
	}
	s.logger.InfoContext(ctx, "return status updated successfully",
		slog.String("return_id", ret.ID),
		slog.String("status", string(ret.Status)),
	)
	return ret, nil
}

func (s *returnService) ApproveReturn(ctx context.Context, id string, reviewerID string) (*model.Return, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrReturnIDRequired
	}
	reviewerID = strings.TrimSpace(reviewerID)
	if reviewerID == "" {
		if u := grpcclient.GetUserID(ctx); u != "" {
			reviewerID = u
		} else if u, ok := auth.UserFromContext(ctx); ok && u != nil {
			reviewerID = u.UserID
		}
	}
	if reviewerID == "" {
		return nil, ErrReviewerIDRequired
	}
	ctx = auth.WithUser(ctx, &auth.UserContext{UserID: reviewerID})
	return s.UpdateReturnStatus(ctx, id, model.ReturnStatusApproved, "", reviewerID)
}

func (s *returnService) RejectReturn(ctx context.Context, id string, rejectionReason string, reviewerID string) (*model.Return, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, ErrReturnIDRequired
	}
	rejectionReason = strings.TrimSpace(rejectionReason)
	if rejectionReason == "" {
		return nil, ErrRejectionReasonRequired
	}
	reviewerID = strings.TrimSpace(reviewerID)
	if reviewerID == "" {
		if u := grpcclient.GetUserID(ctx); u != "" {
			reviewerID = u
		} else if u, ok := auth.UserFromContext(ctx); ok && u != nil {
			reviewerID = u.UserID
		}
	}
	if reviewerID == "" {
		return nil, ErrReviewerIDRequired
	}
	ctx = auth.WithUser(ctx, &auth.UserContext{UserID: reviewerID})
	return s.UpdateReturnStatus(ctx, id, model.ReturnStatusRejected, rejectionReason, reviewerID)
}

func (s *returnService) VerifyProduct(ctx context.Context, req *dto.VerifyProductDTO) (*model.Return, *model.ProductVerification, error) {
	if req == nil {
		return nil, nil, errors.New("request cannot be nil")
	}

	req.ReturnID = strings.TrimSpace(req.ReturnID)
	if req.ReturnID == "" {
		return nil, nil, errors.New("return id is required")
	}

	req.VerifiedBy = strings.TrimSpace(req.VerifiedBy)
	if req.VerifiedBy == "" {
		return nil, nil, ErrVerifiedByRequired
	}

	cond := model.VerificationCondition(strings.ToUpper(strings.TrimSpace(req.Condition)))
	if !cond.IsValid() {
		return nil, nil, ErrInvalidVerificationCondition
	}

	cleanedImages := make([]string, 0, len(req.ProofImages))
	seen := make(map[string]bool)
	for _, img := range req.ProofImages {
		tImg := strings.TrimSpace(img)
		if tImg != "" && !seen[tImg] {
			seen[tImg] = true
			cleanedImages = append(cleanedImages, tImg)
		}
	}

	ret, err := s.repo.GetReturnByID(ctx, req.ReturnID)
	if err != nil {
		if errors.Is(err, repository.ErrReturnNotFound) {
			return nil, nil, ErrReturnNotFound
		}
		return nil, nil, err
	}
	if ret == nil {
		return nil, nil, ErrReturnNotFound
	}

	if ret.Status != model.ReturnStatusRequested &&
		ret.Status != model.ReturnStatusRequestedAlias &&
		ret.Status != model.ReturnStatusReplacementRequested &&
		ret.Status != model.ReturnStatusExchangeRequested &&
		ret.Status != model.ReturnStatusApproved &&
		ret.Status != model.ReturnStatusPickedUp &&
		ret.Status != model.ReturnStatusReceived &&
		ret.Status != model.ReturnStatusItemReceived {
		return nil, nil, ErrReturnNotVerifiable
	}

	now := time.Now().UTC()
	v := &model.ProductVerification{
		ID:          uuid.New().String(),
		ReturnID:    ret.ID,
		VerifiedBy:  req.VerifiedBy,
		Condition:   cond,
		IsApproved:  req.IsApproved,
		ProofImages: cleanedImages,
		Notes:       strings.TrimSpace(req.Notes),
		VerifiedAt:  now,
		CreatedAt:   now,
		UpdatedAt:   now,
	}

	var nextStatus model.ReturnStatus
	if req.IsApproved {
		nextStatus = model.ReturnStatusVerified
	} else {
		nextStatus = model.ReturnStatusRejected
	}

	var outboxEvents []*outbox.Event
	outboxEvt, err := events.BuildReturnVerifiedOutboxEvent(ret, v)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build return verified outbox event", slog.Any("error", err))
		return nil, nil, err
	}
	outboxEvents = append(outboxEvents, outboxEvt)

	if req.IsApproved && ret.Type == model.ReturnTypeReturn {
		eligibleEvt, err := events.BuildReturnRefundEligibleOutboxEvent(ret, "Product verified and approved for refund")
		if err == nil && eligibleEvt != nil {
			outboxEvents = append(outboxEvents, eligibleEvt)
		}
	}

	if err := s.repo.CreateVerificationWithOutbox(ctx, v, nextStatus, outboxEvents...); err != nil {
		s.logger.ErrorContext(ctx, "failed to persist product verification", slog.Any("error", err))
		return nil, nil, err
	}

	ret.Status = nextStatus
	if req.IsApproved && ret.ApprovedAt == nil {
		ret.ApprovedAt = &now
	}
	ret.Verification = v
	ret.UpdatedAt = now

	s.logger.InfoContext(ctx, "product verification completed successfully",
		slog.String("return_id", ret.ID),
		slog.String("verified_by", req.VerifiedBy),
		slog.Bool("is_approved", req.IsApproved),
		slog.String("status", string(nextStatus)),
	)

	if req.IsApproved && ret.Type == model.ReturnTypeReturn && s.paymentClient != nil {
		if refundedRet, refErr := s.TriggerRefund(ctx, ret.ID); refErr == nil && refundedRet != nil {
			ret = refundedRet
			ret.Verification = v
		} else if refErr != nil {
			s.logger.WarnContext(ctx, "automatic refund trigger after product verification deferred/failed",
				slog.String("return_id", ret.ID),
				slog.Any("error", refErr),
			)
		}
	}

	return ret, v, nil
}

func (s *returnService) ListReturns(ctx context.Context, req *dto.ListReturnsDTO) ([]*model.Return, int, error) {
	if req == nil {
		req = &dto.ListReturnsDTO{Limit: 20, Offset: 0}
	}
	return s.repo.ListReturns(ctx, req.Limit, req.Offset, req.Status, req.UserID, req.Type)
}

func (s *returnService) ListReturnsByOrderID(ctx context.Context, orderID string) ([]*model.Return, error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil, errors.New("order id is required")
	}
	return s.repo.ListReturnsByOrderID(ctx, orderID)
}

func parseDeliveryTime(order *orderpb.Order) time.Time {
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02 15:04:05.999999999-07:00",
		"2006-01-02 15:04:05.999999-07:00",
		"2006-01-02 15:04:05-07:00",
		"2006-01-02 15:04:05",
		"2006-01-02T15:04:05",
	}

	for _, ts := range []string{order.GetUpdatedAt(), order.GetCreatedAt()} {
		ts = strings.TrimSpace(ts)
		if ts == "" {
			continue
		}
		for _, layout := range formats {
			if t, err := time.Parse(layout, ts); err == nil {
				return t
			}
		}
	}
	return time.Now().UTC()
}

func (s *returnService) CreateExchange(ctx context.Context, req *dto.CreateExchangeDTO) (*model.Exchange, error) {
	if req == nil {
		return nil, errors.New("request cannot be nil")
	}

	req.UserID = strings.TrimSpace(req.UserID)
	if req.UserID == "" {
		return nil, ErrUserIDRequired
	}
	req.OrderID = strings.TrimSpace(req.OrderID)
	if req.OrderID == "" {
		return nil, ErrOrderIDRequired
	}
	req.NewProductID = strings.TrimSpace(req.NewProductID)
	if req.NewProductID == "" {
		return nil, ErrExchangeNewProductRequired
	}
	req.OldBrand = strings.TrimSpace(req.OldBrand)
	req.OldModel = strings.TrimSpace(req.OldModel)
	if req.OldBrand == "" || req.OldModel == "" {
		return nil, ErrExchangeBrandModelRequired
	}
	if req.BaseExchangeValue < 0 {
		return nil, errors.New("base exchange value cannot be negative")
	}

	if s.orderClient != nil {
		order, err := s.orderClient.GetOrder(ctx, req.OrderID, req.UserID)
		if err != nil {
			s.logger.ErrorContext(ctx, "failed to retrieve order from order service for exchange",
				slog.String("order_id", req.OrderID),
				slog.String("user_id", req.UserID),
				slog.Any("error", err),
			)
			return nil, err
		}
		if order == nil {
			s.logger.WarnContext(ctx, "order not found for exchange request", slog.String("order_id", req.OrderID))
			return nil, ErrOrderNotFound
		}
		if order.GetUserId() != "" && order.GetUserId() != req.UserID {
			s.logger.WarnContext(ctx, "exchange request rejected: unauthorized user",
				slog.String("order_id", req.OrderID),
				slog.String("request_user_id", req.UserID),
				slog.String("order_user_id", order.GetUserId()),
			)
			return nil, ErrUnauthorizedReturnRequest
		}
	}

	totalDeductions := 0.0
	var answers []*model.ExchangeQuestionAnswer
	now := time.Now().UTC()

	for _, a := range req.Answers {
		if a == nil {
			continue
		}
		qKey := strings.TrimSpace(a.QuestionKey)
		ansVal := strings.TrimSpace(a.AnswerValue)
		if qKey == "" {
			continue
		}
		ded := a.DeductionAmount
		if ded < 0 {
			ded = 0
		}
		totalDeductions += ded
		answers = append(answers, &model.ExchangeQuestionAnswer{
			ID:              uuid.New().String(),
			QuestionKey:     qKey,
			AnswerValue:     ansVal,
			DeductionAmount: ded,
			CreatedAt:       now,
		})
	}

	estimatedDiscount := req.BaseExchangeValue - totalDeductions
	if estimatedDiscount < 0 {
		estimatedDiscount = 0
	}

	priceDiff := 0.0
	if s.productClient != nil {
		prod, err := s.productClient.GetProduct(ctx, req.NewProductID)
		if err == nil && prod != nil && prod.GetPrice() > 0 {
			priceDiff = prod.GetPrice() - estimatedDiscount
		}
	}

	exc := &model.Exchange{
		ID:                uuid.New().String(),
		OrderID:           req.OrderID,
		OrderItemID:       strings.TrimSpace(req.OrderItemID),
		UserID:            req.UserID,
		NewProductID:      req.NewProductID,
		OldBrand:          req.OldBrand,
		OldModel:          req.OldModel,
		OldVariant:        strings.TrimSpace(req.OldVariant),
		SerialOrIMEI:      strings.TrimSpace(req.SerialOrIMEI),
		BaseExchangeValue: req.BaseExchangeValue,
		EstimatedDiscount: estimatedDiscount,
		FinalDiscount:     estimatedDiscount,
		PriceDifference:   priceDiff,
		Status:            model.ExchangeStatusOfferCreated,
		Answers:           answers,
		CreatedAt:         now,
		UpdatedAt:         now,
	}

	outboxEvt, err := events.BuildExchangeCreatedOutboxEvent(exc)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build exchange created outbox event", slog.Any("error", err))
		return nil, err
	}

	if err := s.repo.CreateExchangeWithOutbox(ctx, exc, answers, outboxEvt); err != nil {
		s.logger.ErrorContext(ctx, "failed to persist exchange", slog.Any("error", err))
		return nil, err
	}

	s.logger.InfoContext(ctx, "exchange created successfully",
		slog.String("exchange_id", exc.ID),
		slog.String("order_id", exc.OrderID),
		slog.String("user_id", exc.UserID),
		slog.Float64("estimated_discount", exc.EstimatedDiscount),
	)

	return exc, nil
}

func (s *returnService) GetExchange(ctx context.Context, id string) (*model.Exchange, error) {
	id = strings.TrimSpace(id)
	if id == "" {
		return nil, errors.New("exchange id is required")
	}
	exc, err := s.repo.GetExchangeByID(ctx, id)
	if err != nil && !errors.Is(err, repository.ErrExchangeNotFound) {
		s.logger.ErrorContext(ctx, "failed to retrieve exchange by id", slog.String("id", id), slog.Any("error", err))
	}
	return exc, err
}

func (s *returnService) VerifyExchangeProduct(ctx context.Context, req *dto.VerifyExchangeProductDTO) (*model.Exchange, *model.ExchangeVerification, error) {
	if req == nil {
		return nil, nil, errors.New("request cannot be nil")
	}

	req.ExchangeID = strings.TrimSpace(req.ExchangeID)
	if req.ExchangeID == "" {
		return nil, nil, errors.New("exchange id is required")
	}
	req.VerifiedBy = strings.TrimSpace(req.VerifiedBy)
	if req.VerifiedBy == "" {
		return nil, nil, ErrVerifiedByRequired
	}

	condRating := model.ExchangeConditionRating(strings.ToUpper(strings.TrimSpace(req.ConditionRating)))
	if !condRating.IsValid() {
		return nil, nil, ErrInvalidExchangeConditionRating
	}

	exc, err := s.repo.GetExchangeByID(ctx, req.ExchangeID)
	if err != nil {
		if errors.Is(err, repository.ErrExchangeNotFound) {
			return nil, nil, ErrExchangeNotFound
		}
		return nil, nil, err
	}
	if exc == nil {
		return nil, nil, ErrExchangeNotFound
	}

	if exc.Status == model.ExchangeStatusCompleted || exc.Status == model.ExchangeStatusCancelled {
		return nil, nil, ErrExchangeNotVerifiable
	}

	// Validation: Device must power on/be functional and serial/IMEI must match
	if req.IsApproved && (!req.Functional || !req.SerialMatches) {
		return nil, nil, ErrExchangeFunctionalRequired
	}

	now := time.Now().UTC()
	var finalDiscount float64
	var nextStatus model.ExchangeStatus

	if req.IsApproved {
		if req.AdjustedDiscount != nil {
			finalDiscount = *req.AdjustedDiscount
		} else {
			finalDiscount = exc.EstimatedDiscount
		}

		if finalDiscount != exc.EstimatedDiscount {
			nextStatus = model.ExchangeStatusRevaluated
		} else {
			nextStatus = model.ExchangeStatusVerified
		}
	} else {
		finalDiscount = 0
		nextStatus = model.ExchangeStatusCancelled
	}

	cleanedImages := make([]string, 0, len(req.ProofImages))
	seen := make(map[string]bool)
	for _, img := range req.ProofImages {
		tImg := strings.TrimSpace(img)
		if tImg != "" && !seen[tImg] {
			seen[tImg] = true
			cleanedImages = append(cleanedImages, tImg)
		}
	}

	v := &model.ExchangeVerification{
		ID:               uuid.New().String(),
		ExchangeID:       exc.ID,
		VerifiedBy:       req.VerifiedBy,
		ConditionRating:  condRating,
		IsApproved:       req.IsApproved,
		ScreenIntact:     req.ScreenIntact,
		BodyIntact:       req.BodyIntact,
		Functional:       req.Functional,
		SerialMatches:    req.SerialMatches,
		AdjustedDiscount: finalDiscount,
		ProofImages:      cleanedImages,
		Notes:            strings.TrimSpace(req.Notes),
		VerifiedAt:       now,
		CreatedAt:        now,
		UpdatedAt:        now,
	}

	outboxEvt, err := events.BuildExchangeVerifiedOutboxEvent(exc, v)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to build exchange verified outbox event", slog.Any("error", err))
		return nil, nil, err
	}

	if err := s.repo.CreateExchangeVerificationWithOutbox(ctx, v, nextStatus, finalDiscount, outboxEvt); err != nil {
		s.logger.ErrorContext(ctx, "failed to persist exchange verification", slog.Any("error", err))
		return nil, nil, err
	}

	exc.Status = nextStatus
	exc.FinalDiscount = finalDiscount
	exc.Verification = v
	exc.UpdatedAt = now

	s.logger.InfoContext(ctx, "exchange product verified successfully",
		slog.String("exchange_id", exc.ID),
		slog.String("verified_by", req.VerifiedBy),
		slog.Bool("is_approved", req.IsApproved),
		slog.String("status", string(nextStatus)),
		slog.Float64("final_discount", finalDiscount),
	)

	return exc, v, nil
}

func (s *returnService) ProcessRefund(ctx context.Context, returnID string) (*model.Return, error) {
	return s.TriggerRefund(ctx, returnID)
}

func (s *returnService) TriggerRefund(ctx context.Context, returnID string) (*model.Return, error) {
	returnID = strings.TrimSpace(returnID)
	if returnID == "" {
		return nil, errors.New("return id is required")
	}

	ret, err := s.repo.GetReturnByID(ctx, returnID)
	if err != nil {
		if errors.Is(err, repository.ErrReturnNotFound) {
			return nil, ErrReturnNotFound
		}
		return nil, err
	}
	if ret == nil {
		return nil, ErrReturnNotFound
	}

	// 1. Non-monetary return types (REPLACEMENT, EXCHANGE) bypass payment refund calls.
	if ret.Type == model.ReturnTypeReplacement || ret.Type == model.ReturnTypeExchange {
		s.logger.InfoContext(ctx, "non-monetary return type bypasses payment refund calls",
			slog.String("return_id", ret.ID),
			slog.String("type", string(ret.Type)),
		)
		return ret, nil
	}

	// Idempotency: if already completed, return existing completed return
	if ret.Status == model.ReturnStatusCompleted {
		s.logger.InfoContext(ctx, "return is already completed", slog.String("return_id", ret.ID))
		return ret, nil
	}

	// 2. Refund is triggered only after the required return state is reached (VERIFIED / RECEIVED).
	if ret.Status != model.ReturnStatusVerified &&
		ret.Status != model.ReturnStatusReceived &&
		ret.Status != model.ReturnStatusItemReceived &&
		ret.Status != model.ReturnStatusRefundInitiated {
		s.logger.WarnContext(ctx, "refund rejected: return not in verified or received status",
			slog.String("return_id", ret.ID),
			slog.String("status", string(ret.Status)),
		)
		return nil, ErrReturnNotEligibleForRefund
	}

	if s.paymentClient == nil {
		s.logger.WarnContext(ctx, "payment client is not configured; cannot trigger refund via payment service",
			slog.String("return_id", ret.ID),
		)
		return nil, errors.New("payment service client unavailable")
	}

	// 3. Call Payment Service through gRPC (GetPaymentByOrderID)
	pmt, err := s.paymentClient.GetPaymentByOrderID(ctx, ret.OrderID)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to get payment by order id",
			slog.String("order_id", ret.OrderID),
			slog.String("return_id", ret.ID),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("failed to retrieve payment for order %s: %w", ret.OrderID, err)
	}
	if pmt == nil {
		s.logger.WarnContext(ctx, "payment record not found for order",
			slog.String("order_id", ret.OrderID),
			slog.String("return_id", ret.ID),
		)
		return nil, ErrPaymentNotFound
	}

	// 4. Derive refund amount and ensure it cannot exceed total paid amount
	refundAmount := ret.RefundAmount
	if refundAmount <= 0 {
		refundAmount = pmt.Amount
	}
	if refundAmount > pmt.Amount {
		s.logger.WarnContext(ctx, "refund amount exceeds total paid amount",
			slog.String("return_id", ret.ID),
			slog.Float64("refund_amount", refundAmount),
			slog.Float64("paid_amount", pmt.Amount),
		)
		return nil, ErrRefundExceedsPaidAmount
	}

	// 5. Deterministic idempotency key based on return ID and order ID
	idempotencyKey := fmt.Sprintf("refund_%s_%s", ret.ID, ret.OrderID)

	// 6. Return status reflects refund progression: transition to REFUND_INITIATED
	if ret.Status != model.ReturnStatusRefundInitiated {
		refInitEvt, buildErr := events.BuildRefundInitiatedOutboxEvent(ret, pmt.Id, refundAmount, pmt.Currency)
		if buildErr != nil {
			s.logger.ErrorContext(ctx, "failed to build refund initiated outbox event", slog.Any("error", buildErr))
			return nil, buildErr
		}
		statusEvt, buildErr := events.BuildReturnStatusUpdatedOutboxEvent(ret, ret.Status, model.ReturnStatusRefundInitiated)
		if buildErr != nil {
			s.logger.ErrorContext(ctx, "failed to build return status updated outbox event", slog.Any("error", buildErr))
			return nil, buildErr
		}

		updatedRet, updateErr := s.repo.UpdateReturnStatusWithOutbox(ctx, ret.ID, model.ReturnStatusRefundInitiated, "", refInitEvt, statusEvt)
		if updateErr != nil {
			s.logger.ErrorContext(ctx, "failed to update return status to REFUND_INITIATED", slog.Any("error", updateErr))
			return nil, updateErr
		}
		ret = updatedRet
	}

	// 7. Call Payment Service through gRPC (CreateRefund)
	createRefReq := &paymentpb.CreateRefundRequest{
		PaymentId:      pmt.Id,
		Amount:         refundAmount,
		Reason:         ret.Reason,
		IdempotencyKey: idempotencyKey,
	}

	refResp, err := s.paymentClient.CreateRefund(ctx, createRefReq)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to create refund through payment service",
			slog.String("return_id", ret.ID),
			slog.String("payment_id", pmt.Id),
			slog.Any("error", err),
		)
		return nil, fmt.Errorf("payment service CreateRefund failed: %w", err)
	}
	if refResp == nil {
		s.logger.ErrorContext(ctx, "empty refund response from payment service",
			slog.String("return_id", ret.ID),
			slog.String("payment_id", pmt.Id),
		)
		return nil, errors.New("payment service returned empty refund response")
	}

	// 8. Check persisted refund status returned by Payment Service.
	// Only transition the return to COMPLETED when the refund has reached the appropriate successful state (SUCCESS or PROCESSED).
	// Otherwise retain a pending/failed refund state.
	refundStatus := strings.ToUpper(strings.TrimSpace(refResp.GetStatus()))
	if refundStatus != "SUCCESS" && refundStatus != "PROCESSED" {
		s.logger.WarnContext(ctx, "refund has not reached successful state; retaining refund initiated state",
			slog.String("return_id", ret.ID),
			slog.String("refund_id", refResp.GetRefundId()),
			slog.String("refund_status", refundStatus),
		)
		if refundStatus == "FAILED" {
			return ret, ErrRefundFailed
		}
		return ret, nil
	}

	// 9. Transition return status to COMPLETED with completed_at timestamp
	now := time.Now().UTC()
	ret.CompletedAt = &now
	ret.Status = model.ReturnStatusCompleted

	completedEvt, buildErr := events.BuildReturnCompletedOutboxEvent(ret)
	if buildErr != nil {
		s.logger.ErrorContext(ctx, "failed to build return completed outbox event", slog.Any("error", buildErr))
		return nil, buildErr
	}

	statusCompletedEvt, buildErr := events.BuildReturnStatusUpdatedOutboxEvent(ret, model.ReturnStatusRefundInitiated, model.ReturnStatusCompleted)
	if buildErr != nil {
		s.logger.ErrorContext(ctx, "failed to build return status updated outbox event", slog.Any("error", buildErr))
		return nil, buildErr
	}

	completedRet, err := s.repo.UpdateReturnStatusWithOutbox(ctx, ret.ID, model.ReturnStatusCompleted, "", completedEvt, statusCompletedEvt)
	if err != nil {
		s.logger.ErrorContext(ctx, "failed to update return status to COMPLETED", slog.Any("error", err))
		return nil, err
	}
	if completedRet != nil {
		completedRet.CompletedAt = &now
		ret = completedRet
	}

	s.logger.InfoContext(ctx, "return refund completed successfully",
		slog.String("return_id", ret.ID),
		slog.String("refund_id", refResp.GetRefundId()),
		slog.Float64("refund_amount", refundAmount),
		slog.Time("completed_at", now),
	)

	return ret, nil
}
