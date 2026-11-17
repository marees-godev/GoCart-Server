package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	orderpb "github.com/marees-godev/GoCart-Server/contracts/protobuf/order"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/dto"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type OrderUserLookup interface {
	LookupOrderUserID(ctx context.Context, orderID string) (string, error)
}

type grpcOrderUserLookup struct {
	client  orderpb.OrderServiceClient
	timeout time.Duration
}

func NewGRPCOrderUserLookup(client orderpb.OrderServiceClient, timeout time.Duration) OrderUserLookup {
	if client == nil {
		return nil
	}
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	return &grpcOrderUserLookup{
		client:  client,
		timeout: timeout,
	}
}

func (l *grpcOrderUserLookup) LookupOrderUserID(ctx context.Context, orderID string) (string, error) {
	if l == nil || l.client == nil || strings.TrimSpace(orderID) == "" {
		return "", nil
	}
	lookupCtx, cancel := context.WithTimeout(ctx, l.timeout)
	defer cancel()

	resp, err := l.client.GetOrder(lookupCtx, &orderpb.GetOrderRequest{Id: strings.TrimSpace(orderID)})
	if err != nil {
		return "", err
	}
	if resp == nil || resp.GetOrder() == nil {
		return "", nil
	}
	return strings.TrimSpace(resp.GetOrder().GetUserId()), nil
}

type NotificationPipelineService interface {
	ProcessEvent(ctx context.Context, envelope *events.EventEnvelope) (*model.NotificationJob, error)
}

type notificationPipelineService struct {
	templateService     TemplateService
	preferenceService   PreferenceService
	jobRepo             repository.JobRepository
	idempotencyRepo     repository.IdempotencyRepository
	notificationService NotificationService
	orderLookup         OrderUserLookup
	logger              *slog.Logger
}

func NewNotificationPipelineService(
	templateService TemplateService,
	preferenceService PreferenceService,
	jobRepo repository.JobRepository,
	idempotencyRepo repository.IdempotencyRepository,
	logger *slog.Logger,
) NotificationPipelineService {
	if logger == nil {
		logger = slog.Default()
	}
	return &notificationPipelineService{
		templateService:   templateService,
		preferenceService: preferenceService,
		jobRepo:           jobRepo,
		idempotencyRepo:   idempotencyRepo,
		logger:            logger,
	}
}

func NewPushNotificationPipelineService(
	templateService TemplateService,
	preferenceService PreferenceService,
	jobRepo repository.JobRepository,
	idempotencyRepo repository.IdempotencyRepository,
	notificationService NotificationService,
	orderLookup OrderUserLookup,
	logger *slog.Logger,
) NotificationPipelineService {
	if logger == nil {
		logger = slog.Default()
	}
	return &notificationPipelineService{
		templateService:     templateService,
		preferenceService:   preferenceService,
		jobRepo:             jobRepo,
		idempotencyRepo:     idempotencyRepo,
		notificationService: notificationService,
		orderLookup:         orderLookup,
		logger:              logger,
	}
}

type extractedEvent struct {
	userID       string
	recipient    string
	orderID      string
	templateData map[string]interface{}
	pushMeta     map[string]string
}

// IsPushSupportedEvent checks if an event type triggers Web Push notifications.
// Web push notifications are triggered when an order is created, confirmed, shipped, delivered, or cancelled,
// or when a delivery is created.
func IsPushSupportedEvent(eventType string) bool {
	clean := strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(strings.TrimSpace(eventType), ".", ""), "_", ""))
	switch clean {
	case "ordercreated", "orderplaced",
		"orderconfirmed",
		"ordershipped",
		"orderdelivered",
		"ordercancelled", "ordercanceled",
		"orderpaymentfailed",
		"deliverycreated",
		"deliverystatusupdated", "deliverystatuschanged":
		return true
	default:
		return false
	}
}

func (s *notificationPipelineService) ProcessEvent(ctx context.Context, envelope *events.EventEnvelope) (*model.NotificationJob, error) {
	if envelope == nil {
		return nil, appErrors.BadRequest("event envelope is nil")
	}

	eventID := strings.TrimSpace(envelope.EventID)
	if eventID == "" {
		eventID = strings.TrimSpace(envelope.AggregateID)
		if eventID == "" {
			return nil, appErrors.BadRequest("missing event_id in envelope")
		}
	}

	// 1. Idempotency Check
	processed, err := s.idempotencyRepo.IsEventProcessed(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("idempotency check failed: %w", err)
	}
	if processed {
		s.logger.InfoContext(ctx, "Duplicate event detected via processed_events, skipping idempotently",
			"event_id", eventID,
			"event_type", envelope.EventType,
		)
		existingJob, _ := s.jobRepo.GetJobByEventID(ctx, eventID)
		return existingJob, nil
	}

	existingJob, err := s.jobRepo.GetJobByEventID(ctx, eventID)
	if err != nil {
		return nil, fmt.Errorf("failed checking existing job: %w", err)
	}

	// 2. Map domain event to recipient, user_id, and template data
	extracted, supported, mapErr := s.extractEventData(ctx, envelope)
	if mapErr != nil {
		return nil, mapErr
	}
	if !supported {
		s.logger.WarnContext(ctx, "unsupported or unrecognized domain event ignored safely",
			"event_type", envelope.EventType,
			"event_id", eventID,
		)
		return nil, nil
	}

	userID := extracted.userID
	if userID == "" && extracted.orderID != "" && s.orderLookup != nil {
		resolvedUserID, lookupErr := s.orderLookup.LookupOrderUserID(ctx, extracted.orderID)
		if lookupErr != nil {
			s.logger.WarnContext(ctx, "failed to resolve user_id from order-service; continuing without push",
				"event_id", eventID,
				"event_type", envelope.EventType,
				"order_id", extracted.orderID,
				"error", lookupErr,
			)
		} else if resolvedUserID != "" {
			userID = resolvedUserID
			extracted.templateData["UserID"] = resolvedUserID
			if extracted.recipient == "" || extracted.recipient == extracted.orderID {
				extracted.recipient = resolvedUserID
			}
		}
	}

	if extracted.recipient == "" {
		extracted.recipient = userID
	}
	if extracted.recipient == "" {
		extracted.recipient = extracted.orderID
	}
	if extracted.recipient == "" {
		extracted.recipient = envelope.AggregateID
	}

	var uID *string
	if userID != "" {
		trimmed := strings.TrimSpace(userID)
		if _, err := uuid.Parse(trimmed); err == nil {
			uID = &trimmed
		}
	}

	var job *model.NotificationJob
	if existingJob != nil {
		job = existingJob
	} else {
		channel, allowed, err := s.preferenceService.CheckPreferenceAndSelectChannel(ctx, userID, envelope.EventType, model.ChannelEmail)
		if err != nil {
			return nil, fmt.Errorf("channel and preference check failed: %w", err)
		}

		tmpl, err := s.templateService.ResolveTemplateForChannel(ctx, envelope.EventType, channel.ID, channel.Code)
		if err != nil {
			return nil, fmt.Errorf("template resolution failed: %w", err)
		}

		renderedSubject, renderedContent, err := s.templateService.Render(tmpl, extracted.templateData)
		if err != nil {
			return nil, fmt.Errorf("template rendering failed: %w", err)
		}

		status := model.JobStatusPending
		if !allowed {
			status = model.JobStatusOptedOut
			s.logger.InfoContext(ctx, "User has opted out of notification, marking job as OPTED_OUT",
				"user_id", userID,
				"event_type", envelope.EventType,
				"channel", channel.Code,
			)
		}

		newJob := &model.NotificationJob{
			EventID:       eventID,
			UserID:        uID,
			ChannelID:     channel.ID,
			ChannelCode:   channel.Code,
			TemplateID:    tmpl.ID,
			EventType:     envelope.EventType,
			Recipient:     truncateString(extracted.recipient, 255),
			Subject:       truncateString(renderedSubject, 255),
			Content:       renderedContent,
			Status:        status,
			RetryCount:    0,
			MaxRetries:    3,
			CorrelationID: envelope.CorrelationID,
		}

		if err := s.jobRepo.CreateJob(ctx, newJob); err != nil {
			if strings.Contains(err.Error(), "already exists") {
				job, _ = s.jobRepo.GetJobByEventID(ctx, eventID)
			} else {
				return nil, fmt.Errorf("failed to create notification job: %w", err)
			}
		} else {
			job = newJob
		}
	}

	// 3. Persist notification history and deliver Web Push for authenticated users on supported events (Order Placed or Delivery Created)
	if s.notificationService != nil && uID != nil && IsPushSupportedEvent(envelope.EventType) {
		pushTitle, pushBody := s.renderPushContent(ctx, envelope.EventType, extracted.templateData, job)
		if extracted.pushMeta == nil {
			extracted.pushMeta = make(map[string]string)
		}
		extracted.pushMeta["event_id"] = eventID
		extracted.pushMeta["event_type"] = envelope.EventType

		_, sendErr := s.notificationService.SendNotification(ctx, dto.SendNotificationInput{
			UserID:        *uID,
			Channel:       model.ChannelPush,
			EventType:     envelope.EventType,
			EventID:       eventID,
			Recipient:     *uID,
			Subject:       pushTitle,
			Content:       pushBody,
			TemplateData:  extracted.pushMeta,
			CorrelationID: envelope.CorrelationID,
		})
		if sendErr != nil {
			return nil, fmt.Errorf("push notification processing failed: %w", sendErr)
		}
	}

	_ = s.idempotencyRepo.RecordProcessedEvent(ctx, &model.ProcessedEvent{
		EventID:   eventID,
		EventType: envelope.EventType,
		Source:    envelope.Source,
	})

	if job != nil {
		s.logger.InfoContext(ctx, "Notification event processed",
			"job_id", job.ID,
			"event_id", eventID,
			"event_type", job.EventType,
			"channel", job.ChannelCode,
			"recipient", job.Recipient,
			"status", job.Status,
		)
	}

	return job, nil
}

func (s *notificationPipelineService) renderPushContent(
	ctx context.Context,
	eventType string,
	templateData map[string]interface{},
	fallbackJob *model.NotificationJob,
) (string, string) {
	if s.templateService != nil {
		if pushTmpl, err := s.templateService.ResolveTemplateForChannel(ctx, eventType, "", model.ChannelPush); err == nil && pushTmpl != nil {
			if subj, body, rErr := s.templateService.Render(pushTmpl, templateData); rErr == nil && subj != "" && body != "" {
				return subj, body
			}
		}
	}
	if fallbackJob != nil {
		return fallbackJob.Subject, fallbackJob.Content
	}
	return eventType, eventType
}

func (s *notificationPipelineService) extractEventData(ctx context.Context, envelope *events.EventEnvelope) (*extractedEvent, bool, error) {
	res := &extractedEvent{
		templateData: make(map[string]interface{}),
		pushMeta:     make(map[string]string),
	}

	switch envelope.EventType {
	case events.EventTypeUserRegistered:
		var payload events.UserRegisteredEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed UserRegisteredEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed UserRegisteredEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.Email, payload.UserID)
		res.templateData["UserID"] = payload.UserID
		res.templateData["Email"] = payload.Email
		res.templateData["Role"] = payload.Role
		res.templateData["BusinessName"] = payload.BusinessName
		res.templateData["Phone"] = payload.Phone

	case events.EventTypeStoreApproved:
		var payload events.StoreApprovedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed StoreApprovedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed StoreApprovedEvent payload")
		}
		res.userID = payload.MerchantID
		res.recipient = payload.MerchantID
		res.templateData["StoreID"] = payload.StoreID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["AdminID"] = payload.AdminID
		res.templateData["ApprovedAt"] = payload.ApprovedAt
		res.pushMeta["store_id"] = payload.StoreID

	case events.EventTypeStoreRejected:
		var payload events.StoreRejectedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed StoreRejectedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed StoreRejectedEvent payload")
		}
		res.userID = payload.MerchantID
		res.recipient = payload.MerchantID
		res.templateData["StoreID"] = payload.StoreID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["AdminID"] = payload.AdminID
		res.templateData["Reason"] = payload.Reason
		res.templateData["RejectedAt"] = payload.RejectedAt
		res.pushMeta["store_id"] = payload.StoreID

	case events.EventTypeMerchantApproved:
		var payload events.MerchantApprovedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed MerchantApprovedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed MerchantApprovedEvent payload")
		}
		res.userID = firstNonEmpty(payload.UserID, payload.MerchantID)
		res.recipient = res.userID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["UserID"] = res.userID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["ApprovedBy"] = payload.ApprovedBy
		res.templateData["Reason"] = payload.Reason
		res.templateData["ApprovedAt"] = payload.ApprovedAt
		res.pushMeta["merchant_id"] = payload.MerchantID

	case events.EventTypeMerchantActivated:
		var payload events.MerchantActivatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed MerchantActivatedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed MerchantActivatedEvent payload")
		}
		res.userID = firstNonEmpty(payload.UserID, payload.MerchantID)
		res.recipient = res.userID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["UserID"] = res.userID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["ActivatedBy"] = payload.ActivatedBy
		res.templateData["Reason"] = payload.Reason
		res.templateData["ActivatedAt"] = payload.ActivatedAt
		res.pushMeta["merchant_id"] = payload.MerchantID

	case events.EventTypeMerchantRejected:
		var payload events.MerchantRejectedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed MerchantRejectedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed MerchantRejectedEvent payload")
		}
		res.userID = firstNonEmpty(payload.UserID, payload.MerchantID)
		res.recipient = res.userID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["UserID"] = res.userID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["RejectedBy"] = payload.RejectedBy
		res.templateData["Reason"] = payload.Reason
		res.templateData["RejectedAt"] = payload.RejectedAt
		res.pushMeta["merchant_id"] = payload.MerchantID

	case events.EventTypeMerchantSuspended:
		var payload events.MerchantSuspendedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed MerchantSuspendedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed MerchantSuspendedEvent payload")
		}
		res.userID = firstNonEmpty(payload.UserID, payload.MerchantID)
		res.recipient = res.userID
		res.templateData["MerchantID"] = payload.MerchantID
		res.templateData["UserID"] = res.userID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["SuspendedBy"] = payload.SuspendedBy
		res.templateData["Reason"] = payload.Reason
		res.templateData["SuspendedAt"] = payload.SuspendedAt
		res.pushMeta["merchant_id"] = payload.MerchantID

	case events.EventTypeOrderCreated, "order.created", "order.placed", "OrderPlaced":
		var payload events.OrderCreatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderCreatedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderCreatedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = payload.UserID
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["TotalAmount"] = payload.TotalAmount
		res.templateData["Currency"] = payload.Currency
		res.templateData["Status"] = payload.Status
		res.pushMeta["order_id"] = payload.OrderID
		if payload.TotalAmount > 0 {
			res.pushMeta["total_amount"] = fmt.Sprintf("%.2f", payload.TotalAmount)
		}
		if payload.Currency != "" {
			res.pushMeta["currency"] = payload.Currency
		}

	case events.EventTypeOrderConfirmed, "order.confirmed", "order_confirmed":
		var payload events.OrderConfirmedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderConfirmedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderConfirmedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["PaymentStatus"] = payload.PaymentStatus
		res.templateData["ConfirmedAt"] = payload.ConfirmedAt
		res.pushMeta["order_id"] = payload.OrderID

	case events.EventTypeOrderCancelled, "order.cancelled", "order.canceled", "order_cancelled", "order_canceled", "OrderCanceled":
		var payload events.OrderCancelledEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderCancelledEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderCancelledEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["Reason"] = payload.Reason
		res.templateData["CancelledAt"] = payload.CancelledAt
		res.pushMeta["order_id"] = payload.OrderID

	case events.EventTypeOrderPaymentFailed, "order.payment_failed", "order.payment-failed":
		var payload events.OrderPaymentFailedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderPaymentFailedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderPaymentFailedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["PaymentID"] = payload.PaymentID
		res.templateData["Reason"] = payload.Reason
		res.templateData["FailedAt"] = payload.FailedAt
		res.pushMeta["order_id"] = payload.OrderID
		if payload.PaymentID != "" {
			res.pushMeta["payment_id"] = payload.PaymentID
		}

	case events.EventTypePaymentSuccessful:
		var payload events.PaymentSuccessfulEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed PaymentSuccessfulEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed PaymentSuccessfulEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = payload.UserID
		res.orderID = payload.OrderID
		res.templateData["PaymentID"] = payload.PaymentID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["Amount"] = payload.Amount
		res.templateData["Currency"] = payload.Currency
		res.templateData["PaymentMethod"] = payload.PaymentMethod
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["payment_id"] = payload.PaymentID

	case events.EventTypePaymentFailed:
		var payload events.PaymentFailedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed PaymentFailedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed PaymentFailedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = payload.UserID
		res.orderID = payload.OrderID
		res.templateData["PaymentID"] = payload.PaymentID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["Amount"] = payload.Amount
		res.templateData["Reason"] = payload.Reason
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["payment_id"] = payload.PaymentID

	case events.EventTypeOrderShipped, "order.shipped", "order_shipped":
		var payload events.OrderShippedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderShippedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderShippedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["DeliveryID"] = payload.DeliveryID
		res.templateData["TrackingNumber"] = payload.TrackingNumber
		res.templateData["DeliveryPartner"] = payload.DeliveryPartner
		res.templateData["ShippedAt"] = payload.ShippedAt
		res.pushMeta["order_id"] = payload.OrderID
		if payload.TrackingNumber != "" {
			res.pushMeta["tracking_number"] = payload.TrackingNumber
		}

	case events.EventTypeOrderDelivered, "order.delivered", "order_delivered":
		var payload events.OrderDeliveredEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderDeliveredEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed OrderDeliveredEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["DeliveryID"] = payload.DeliveryID
		res.templateData["TrackingNumber"] = payload.TrackingNumber
		res.templateData["DeliveryPartner"] = payload.DeliveryPartner
		res.templateData["DeliveredAt"] = payload.DeliveredAt
		res.pushMeta["order_id"] = payload.OrderID

	case events.EventTypeDeliveryCreated, "delivery.created", "delivery_created":
		var payload events.DeliveryCreatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed DeliveryCreatedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed DeliveryCreatedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["DeliveryID"] = payload.DeliveryID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["DeliveryPartner"] = payload.DeliveryPartner
		res.templateData["TrackingNumber"] = payload.TrackingNumber
		res.templateData["Status"] = payload.Status
		res.templateData["CreatedAt"] = payload.CreatedAt
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["delivery_id"] = payload.DeliveryID
		res.pushMeta["tracking_number"] = payload.TrackingNumber

	case events.EventTypeDeliveryStatusUpdated:
		var payload events.DeliveryStatusUpdatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed DeliveryStatusUpdatedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed DeliveryStatusUpdatedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["DeliveryID"] = payload.DeliveryID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["TrackingNumber"] = payload.TrackingNumber
		res.templateData["UpdatedAt"] = payload.UpdatedAt
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["delivery_id"] = payload.DeliveryID
		res.pushMeta["tracking_number"] = payload.TrackingNumber

	case events.EventTypeDeliveryStatusChanged:
		var payload events.DeliveryStatusChangedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed DeliveryStatusChangedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed DeliveryStatusChangedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID)
		res.orderID = payload.OrderID
		res.templateData["DeliveryID"] = payload.DeliveryID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["UserID"] = payload.UserID
		res.templateData["PreviousStatus"] = payload.PreviousStatus
		res.templateData["NewStatus"] = payload.NewStatus
		res.templateData["TrackingNumber"] = payload.TrackingNumber
		res.templateData["UpdatedAt"] = payload.UpdatedAt
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["delivery_id"] = payload.DeliveryID
		res.pushMeta["tracking_number"] = payload.TrackingNumber

	case events.EventTypeReturnApproved:
		var payload events.ReturnApprovedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed ReturnApprovedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed ReturnApprovedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = payload.UserID
		res.orderID = payload.OrderID
		res.templateData["ReturnID"] = payload.ReturnID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["OrderItemID"] = payload.OrderItemID
		res.templateData["UserID"] = payload.UserID
		res.templateData["RefundAmount"] = payload.RefundAmount
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["return_id"] = payload.ReturnID

	case events.EventTypeReturnRejected:
		var payload events.ReturnRejectedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed ReturnRejectedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed ReturnRejectedEvent payload")
		}
		res.userID = payload.UserID
		res.recipient = payload.UserID
		res.orderID = payload.OrderID
		res.templateData["ReturnID"] = payload.ReturnID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["OrderItemID"] = payload.OrderItemID
		res.templateData["UserID"] = payload.UserID
		res.templateData["RejectionReason"] = payload.RejectionReason
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["return_id"] = payload.ReturnID

	case events.EventTypeRefundCompleted:
		var payload events.RefundCompletedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed RefundCompletedEvent payload, routing to dlq", "event_id", envelope.EventID, "error", err)
			return nil, false, appErrors.BadRequest("malformed RefundCompletedEvent payload")
		}
		res.userID = payload.UserID
		if res.userID == "" && envelope.AggregateID != "" {
			if _, err := uuid.Parse(envelope.AggregateID); err == nil {
				res.userID = envelope.AggregateID
			}
		}
		res.recipient = firstNonEmpty(payload.UserID, payload.OrderID, envelope.AggregateID)
		res.orderID = payload.OrderID
		res.templateData["RefundID"] = payload.RefundID
		res.templateData["PaymentID"] = payload.PaymentID
		res.templateData["OrderID"] = payload.OrderID
		res.templateData["Amount"] = payload.Amount
		res.templateData["Currency"] = payload.Currency
		res.templateData["GatewayRefundID"] = payload.GatewayRefundID
		res.pushMeta["order_id"] = payload.OrderID
		res.pushMeta["refund_id"] = payload.RefundID

	default:
		return nil, false, nil
	}

	return res, true, nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if trimmed := strings.TrimSpace(v); trimmed != "" {
			return trimmed
		}
	}
	return ""
}
