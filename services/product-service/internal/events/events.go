package events

import (
	"time"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/product-service/internal/model"
)

const SourceProductService = "product-service"

func BuildProductCreatedOutboxEvent(p *model.Product) (*outbox.Event, error) {
	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}
	payload := events.ProductCreatedEvent{
		ProductID:   p.ID,
		StoreID:     p.StoreID,
		CategoryID:  p.CategoryID,
		SKU:         p.SKU,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Currency:    currency,
		Status:      string(p.Status),
		CreatedAt:   p.CreatedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductCreated, SourceProductService, p.ID, payload)
	if err != nil {
		return nil, err
	}
	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}
	return &outbox.Event{
		AggregateType: "product",
		AggregateID:   p.ID,
		EventType:     events.EventTypeProductCreated,
		Payload:       bytes,
		Topic:         events.TopicProductCreated,
	}, nil
}

func BuildProductUpdatedOutboxEvent(p *model.Product) (*outbox.Event, error) {
	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}
	payload := events.ProductUpdatedEvent{
		ProductID:   p.ID,
		StoreID:     p.StoreID,
		CategoryID:  p.CategoryID,
		SKU:         p.SKU,
		Name:        p.Name,
		Description: p.Description,
		Price:       p.Price,
		Currency:    currency,
		Status:      string(p.Status),
		UpdatedAt:   p.UpdatedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductUpdated, SourceProductService, p.ID, payload)
	if err != nil {
		return nil, err
	}
	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}
	return &outbox.Event{
		AggregateType: "product",
		AggregateID:   p.ID,
		EventType:     events.EventTypeProductUpdated,
		Payload:       bytes,
		Topic:         events.TopicProductUpdated,
	}, nil
}

func BuildProductPublishedOutboxEvent(p *model.Product, publishedAt time.Time) (*outbox.Event, error) {
	currency := p.Currency
	if currency == "" {
		currency = "USD"
	}
	payload := events.ProductPublishedEvent{
		ProductID:   p.ID,
		StoreID:     p.StoreID,
		CategoryID:  p.CategoryID,
		SKU:         p.SKU,
		Name:        p.Name,
		Price:       p.Price,
		Currency:    currency,
		PublishedAt: publishedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductPublished, SourceProductService, p.ID, payload)
	if err != nil {
		return nil, err
	}
	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}
	return &outbox.Event{
		AggregateType: "product",
		AggregateID:   p.ID,
		EventType:     events.EventTypeProductPublished,
		Payload:       bytes,
		Topic:         events.TopicProductPublished,
	}, nil
}

func BuildProductDeletedOutboxEvent(productID, storeID string, deletedAt time.Time) (*outbox.Event, error) {
	payload := events.ProductDeletedEvent{
		ProductID: productID,
		StoreID:   storeID,
		Status:    string(model.StatusDeleted),
		DeletedAt: deletedAt,
	}
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypeProductDeleted, SourceProductService, productID, payload)
	if err != nil {
		return nil, err
	}
	bytes, err := env.Marshal()
	if err != nil {
		return nil, err
	}
	return &outbox.Event{
		AggregateType: "product",
		AggregateID:   productID,
		EventType:     events.EventTypeProductDeleted,
		Payload:       bytes,
		Topic:         events.TopicProductDeleted,
	}, nil
}
