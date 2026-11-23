package consumer

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/services/personalization-service/internal/service"
)

type ActivityConsumer struct {
	svc service.PersonalizationService
	log *slog.Logger
}

func NewActivityConsumer(svc service.PersonalizationService, log *slog.Logger) *ActivityConsumer {
	return &ActivityConsumer{
		svc: svc,
		log: log,
	}
}

// HandleEvent matches the signature expected by pkg/kafka.Consumer
func (c *ActivityConsumer) HandleEvent(ctx context.Context, envelope *events.EventEnvelope) (err error) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error("Recovered from panic during activity event handling", "panic", r)
			err = fmt.Errorf("panic in event handler: %v", r)
		}
	}()

	if envelope == nil {
		c.log.Warn("Received nil event envelope, ignoring safely")
		return nil
	}

	return c.svc.ProcessEvent(ctx, envelope)
}