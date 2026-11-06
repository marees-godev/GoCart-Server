package service

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/repository"
)

type NotificationPipelineService interface {
	ProcessEvent(ctx context.Context, envelope *events.EventEnvelope) (*model.NotificationJob, error)
}

type notificationPipelineService struct {
	templateService   TemplateService
	preferenceService PreferenceService
	jobRepo           repository.JobRepository
	idempotencyRepo   repository.IdempotencyRepository
	logger            *slog.Logger
}

func NewNotificationPipelineService(
	templateService TemplateService,
	preferenceService PreferenceService,
	jobRepo repository.JobRepository,
	idempotencyRepo repository.IdempotencyRepository,
	logger *slog.Logger,
) NotificationPipelineService {
	return &notificationPipelineService{
		templateService:   templateService,
		preferenceService: preferenceService,
		jobRepo:           jobRepo,
		idempotencyRepo:   idempotencyRepo,
		logger:            logger,
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
	if existingJob != nil {
		s.logger.InfoContext(ctx, "Duplicate event detected via notification_jobs, skipping idempotently",
			"event_id", eventID,
			"event_type", envelope.EventType,
		)
		_ = s.idempotencyRepo.RecordProcessedEvent(ctx, &model.ProcessedEvent{
			EventID:   eventID,
			EventType: envelope.EventType,
			Source:    envelope.Source,
		})
		return existingJob, nil
	}

	// 2. Map domain event to recipient, user_id, and template data
	var (
		recipient    string
		userID       string
		templateData = make(map[string]interface{})
		channelCode  = "EMAIL"
	)

	switch envelope.EventType {
	case events.EventTypeUserRegistered:
		var payload events.UserRegisteredEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed UserRegisteredEvent payload, routing to dlq/skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.Email
		if recipient == "" {
			recipient = payload.UserID
		}
		templateData["UserID"] = payload.UserID
		templateData["Email"] = payload.Email
		templateData["Role"] = payload.Role
		templateData["BusinessName"] = payload.BusinessName
		templateData["Phone"] = payload.Phone

	case events.EventTypeStoreApproved:
		var payload events.StoreApprovedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed StoreApprovedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.MerchantID
		recipient = payload.MerchantID
		templateData["StoreID"] = payload.StoreID
		templateData["MerchantID"] = payload.MerchantID
		templateData["AdminID"] = payload.AdminID
		templateData["ApprovedAt"] = payload.ApprovedAt

	case events.EventTypeStoreRejected:
		var payload events.StoreRejectedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed StoreRejectedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.MerchantID
		recipient = payload.MerchantID
		templateData["StoreID"] = payload.StoreID
		templateData["MerchantID"] = payload.MerchantID
		templateData["AdminID"] = payload.AdminID
		templateData["Reason"] = payload.Reason
		templateData["RejectedAt"] = payload.RejectedAt

	case events.EventTypeOrderCreated:
		var payload events.OrderCreatedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderCreatedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["OrderID"] = payload.OrderID
		templateData["UserID"] = payload.UserID
		templateData["TotalAmount"] = payload.TotalAmount
		templateData["Currency"] = payload.Currency
		templateData["Status"] = payload.Status

	case events.EventTypePaymentSuccessful:
		var payload events.PaymentSuccessfulEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed PaymentSuccessfulEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["PaymentID"] = payload.PaymentID
		templateData["OrderID"] = payload.OrderID
		templateData["UserID"] = payload.UserID
		templateData["Amount"] = payload.Amount
		templateData["Currency"] = payload.Currency
		templateData["PaymentMethod"] = payload.PaymentMethod

	case events.EventTypePaymentFailed:
		var payload events.PaymentFailedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed PaymentFailedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["PaymentID"] = payload.PaymentID
		templateData["OrderID"] = payload.OrderID
		templateData["UserID"] = payload.UserID
		templateData["Amount"] = payload.Amount
		templateData["Reason"] = payload.Reason

	case events.EventTypeOrderShipped:
		var payload events.OrderShippedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderShippedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["OrderID"] = payload.OrderID
		templateData["UserID"] = payload.UserID
		templateData["TrackingNumber"] = payload.TrackingNumber
		templateData["DeliveryPartner"] = payload.DeliveryPartner
		templateData["ShippedAt"] = payload.ShippedAt

	case events.EventTypeOrderDelivered:
		var payload events.OrderDeliveredEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed OrderDeliveredEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["OrderID"] = payload.OrderID
		templateData["UserID"] = payload.UserID
		templateData["DeliveredAt"] = payload.DeliveredAt

	case events.EventTypeReturnApproved:
		var payload events.ReturnApprovedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed ReturnApprovedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["ReturnID"] = payload.ReturnID
		templateData["OrderID"] = payload.OrderID
		templateData["OrderItemID"] = payload.OrderItemID
		templateData["UserID"] = payload.UserID
		templateData["RefundAmount"] = payload.RefundAmount

	case events.EventTypeReturnRejected:
		var payload events.ReturnRejectedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed ReturnRejectedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		recipient = payload.UserID
		templateData["ReturnID"] = payload.ReturnID
		templateData["OrderID"] = payload.OrderID
		templateData["OrderItemID"] = payload.OrderItemID
		templateData["UserID"] = payload.UserID
		templateData["RejectionReason"] = payload.RejectionReason

	case events.EventTypeRefundCompleted:
		var payload events.RefundCompletedEvent
		if err := envelope.UnmarshalData(&payload); err != nil {
			s.logger.WarnContext(ctx, "malformed RefundCompletedEvent payload, skipping safely", "error", err)
			return nil, nil
		}
		userID = payload.UserID
		if userID == "" && envelope.AggregateID != "" {
			if _, err := uuid.Parse(envelope.AggregateID); err == nil {
				userID = envelope.AggregateID
			}
		}
		recipient = payload.UserID
		if recipient == "" {
			recipient = payload.OrderID
			if recipient == "" {
				recipient = envelope.AggregateID
			}
		}
		templateData["RefundID"] = payload.RefundID
		templateData["PaymentID"] = payload.PaymentID
		templateData["OrderID"] = payload.OrderID
		templateData["Amount"] = payload.Amount
		templateData["Currency"] = payload.Currency
		templateData["GatewayRefundID"] = payload.GatewayRefundID

	default:
		s.logger.WarnContext(ctx, "unsupported or unrecognized domain event ignored safely",
			"event_type", envelope.EventType,
			"event_id", eventID,
		)
		return nil, nil
	}

	// 3. Check Preferences & Select Channel
	channel, allowed, err := s.preferenceService.CheckPreferenceAndSelectChannel(ctx, userID, envelope.EventType, channelCode)
	if err != nil {
		return nil, fmt.Errorf("channel and preference check failed: %w", err)
	}

	// 4. Resolve Template
	tmpl, err := s.templateService.ResolveTemplate(ctx, envelope.EventType, channel.ID)
	if err != nil {
		return nil, fmt.Errorf("template resolution failed: %w", err)
	}

	renderedSubject, renderedContent, err := s.templateService.Render(tmpl, templateData)
	if err != nil {
		return nil, fmt.Errorf("template rendering failed: %w", err)
	}

	// 5. Create Notification Job
	status := model.JobStatusPending
	if !allowed {
		status = model.JobStatusOptedOut
		s.logger.InfoContext(ctx, "User has opted out of notification, marking job as OPTED_OUT",
			"user_id", userID,
			"event_type", envelope.EventType,
			"channel", channel.Code,
		)
	}

	var uID *string
	if userID != "" {
		trimmed := strings.TrimSpace(userID)
		if _, err := uuid.Parse(trimmed); err == nil {
			uID = &trimmed
		}
	}

	job := &model.NotificationJob{
		EventID:       eventID,
		UserID:        uID,
		ChannelID:     channel.ID,
		ChannelCode:   channel.Code,
		TemplateID:    tmpl.ID,
		EventType:     envelope.EventType,
		Recipient:     recipient,
		Subject:       renderedSubject,
		Content:       renderedContent,
		Status:        status,
		RetryCount:    0,
		MaxRetries:    3,
		CorrelationID: envelope.CorrelationID,
	}

	if err := s.jobRepo.CreateJob(ctx, job); err != nil {
		if strings.Contains(err.Error(), "already exists") {
			existing, _ := s.jobRepo.GetJobByEventID(ctx, eventID)
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create notification job: %w", err)
	}

	_ = s.idempotencyRepo.RecordProcessedEvent(ctx, &model.ProcessedEvent{
		EventID:   eventID,
		EventType: envelope.EventType,
		Source:    envelope.Source,
	})

	s.logger.InfoContext(ctx, "Notification job created successfully",
		"job_id", job.ID,
		"event_type", job.EventType,
		"channel", channel.Code,
		"recipient", job.Recipient,
		"status", job.Status,
	)

	return job, nil
}
