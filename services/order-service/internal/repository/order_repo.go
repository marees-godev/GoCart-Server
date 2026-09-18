package repository

import (
	"context"

	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/database"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/jackc/pgx/v5"
)

// OrderRepo is the reference implementation showing how to use the Transactional
// Outbox pattern. Any repository that needs to publish a domain event should
// follow this same structure:
//
//  1. Accept an outbox.Store and database.DB from the service layer.
//  2. Wrap all writes in db.WithTransaction.
//  3. Insert the business record and the outbox event in the SAME transaction.
//
// The outbox publisher (running in a separate goroutine) will pick up PENDING
// events and publish them to Kafka after this transaction commits. If the
// transaction rolls back for any reason, no outbox event will exist — the "DB
// updated but event lost" scenario is structurally impossible.
type OrderRepo struct {
	db          *database.DB
	outboxStore *outbox.Store
}

// NewOrderRepo constructs an OrderRepo.
func NewOrderRepo(db *database.DB, outboxStore *outbox.Store) *OrderRepo {
	return &OrderRepo{db: db, outboxStore: outboxStore}
}

// CreateOrderWithOutbox persists a new order and records the OrderCreated domain
// event in the outbox within a single atomic transaction.
//
// Parameters:
//   - orderID: the UUID of the order being created (as a string)
//   - insertOrder: a function that inserts the order business record using tx
//   - envelope: the fully constructed EventEnvelope to be published
//   - topic: the Kafka topic the outbox publisher should route the event to
func (r *OrderRepo) CreateOrderWithOutbox(
	ctx context.Context,
	orderID string,
	insertOrder func(ctx context.Context, tx pgx.Tx) error,
	envelope *events.EventEnvelope,
	topic string,
) error {
	return r.db.WithTransaction(ctx, func(tx pgx.Tx) error {
		// Step 1 — persist the business record.
		if err := insertOrder(ctx, tx); err != nil {
			return err
		}

		// Step 2 — persist the outbox event in the same transaction.
		payload, err := envelope.Marshal()
		if err != nil {
			return err
		}

		evt := &outbox.Event{
			AggregateType: "order",
			AggregateID:   orderID,
			EventType:     envelope.EventType,
			Payload:       payload,
			Topic:         topic,
		}

		return r.outboxStore.Insert(ctx, tx, evt)
	})
}
