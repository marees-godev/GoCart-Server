package consumer

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/logger"
	"github.com/marees-godev/GoCart-Server/services/notification-service/internal/service"
)

type NotificationEventConsumer struct {
	pipelineService service.NotificationPipelineService
	logger          *slog.Logger
}

func NewNotificationEventConsumer(
	pipelineService service.NotificationPipelineService,
	logger *slog.Logger,
) *NotificationEventConsumer {
	return &NotificationEventConsumer{
		pipelineService: pipelineService,
		logger:          logger,
	}
}

// HandleEvent processes incoming domain events from Kafka topics.
func (c *NotificationEventConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) (err error) {
	// Defensive recovery against unexpected panics
	defer func() {
		if r := recover(); r != nil {
			c.logger.ErrorContext(ctx, "recovered from panic during notification event handling",
				"panic", r,
			)
			err = fmt.Errorf("panic in event handler: %v", r)
		}
	}()

	if envelope == nil {
		c.logger.WarnContext(ctx, "received nil event envelope, ignoring safely")
		return nil
	}

	// Propagate Correlation ID and Request ID into context
	if envelope.CorrelationID != "" {
		ctx = logger.WithCorrelationID(ctx, envelope.CorrelationID)
		ctx = logger.WithRequestID(ctx, envelope.CorrelationID)
	}

	c.logger.InfoContext(ctx, "Received domain event in Notification Service",
		"event_id", envelope.EventID,
		"event_type", envelope.EventType,
		"source", envelope.Source,
		"correlation_id", envelope.CorrelationID,
	)

	_, err = c.pipelineService.ProcessEvent(ctx, envelope)
	if err != nil {
		var appErr *appErrors.AppError
		if errors.As(err, &appErr) && appErr.Code == appErrors.CodeBadRequest {
			c.logger.WarnContext(ctx, "bad request during event processing, acknowledging without retry to avoid poison pill",
				"event_id", envelope.EventID,
				"error", err,
			)
			return nil
		}
		c.logger.ErrorContext(ctx, "transient error processing event, triggering retry",
			"event_id", envelope.EventID,
			"error", err,
		)
		return err
	}

	return nil
}
