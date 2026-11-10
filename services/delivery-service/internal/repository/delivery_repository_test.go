package repository

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	appErrors "github.com/marees-godev/GoCart-Server/pkg/errors"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/delivery-service/internal/model"
)

type recordedOutboxRow struct {
	AggregateType string
	AggregateID   string
	EventType     string
	Payload       []byte
	Topic         string
}

type fakeDeliveryRow struct {
	delivery *model.Delivery
	err      error
}

func (r *fakeDeliveryRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	d := r.delivery
	*(dest[0].(*string)) = d.ID
	*(dest[1].(*string)) = d.OrderID
	*(dest[2].(*string)) = d.DeliveryPartner
	*(dest[3].(*string)) = d.TrackingNumber
	*(dest[4].(*model.DeliveryStatus)) = d.Status
	*(dest[5].(*string)) = d.ShippingAddress.Street
	*(dest[6].(*string)) = d.ShippingAddress.City
	*(dest[7].(*string)) = d.ShippingAddress.State
	*(dest[8].(*string)) = d.ShippingAddress.PostalCode
	*(dest[9].(*string)) = d.ShippingAddress.Country
	*(dest[10].(*string)) = d.ShippingAddress.ContactNumber
	*(dest[11].(*[]byte)) = []byte(`{}`)
	*(dest[12].(**uuid.UUID)) = nil
	*(dest[13].(**time.Time)) = d.EstimatedDelivery
	*(dest[14].(**time.Time)) = d.ActualDelivery
	*(dest[15].(*time.Time)) = d.CreatedAt
	*(dest[16].(*time.Time)) = d.UpdatedAt
	return nil
}

type fakeTx struct {
	parent          *fakeTx
	existing        *model.Delivery
	failOutboxEvent string
	failOutboxErr   error
	insertedOutbox  []recordedOutboxRow
	committed       bool
	rolledBack      bool
}

func (t *fakeTx) root() *fakeTx {
	if t.parent != nil {
		return t.parent.root()
	}
	return t
}

func (t *fakeTx) Begin(ctx context.Context) (pgx.Tx, error) {
	return &fakeTx{parent: t}, nil
}

func (t *fakeTx) Commit(ctx context.Context) error {
	if t.parent != nil {
		return nil
	}
	t.committed = true
	return nil
}

func (t *fakeTx) Rollback(ctx context.Context) error {
	if t.parent != nil {
		return nil
	}
	if !t.committed {
		t.rolledBack = true
	}
	return nil
}

func (t *fakeTx) CopyFrom(ctx context.Context, tableName pgx.Identifier, columnNames []string, rowSrc pgx.CopyFromSource) (int64, error) {
	return 0, nil
}

func (t *fakeTx) SendBatch(ctx context.Context, b *pgx.Batch) pgx.BatchResults {
	return nil
}

func (t *fakeTx) LargeObjects() pgx.LargeObjects {
	return pgx.LargeObjects{}
}

func (t *fakeTx) Prepare(ctx context.Context, name, sql string) (*pgconn.StatementDescription, error) {
	return nil, nil
}

func (t *fakeTx) Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error) {
	root := t.root()
	if strings.Contains(sql, "INSERT INTO outbox_events") {
		var aggType, aggID, eventType, topic string
		var payload []byte
		if len(arguments) >= 7 {
			aggType, _ = arguments[1].(string)
			aggID, _ = arguments[2].(string)
			eventType, _ = arguments[3].(string)
			payload, _ = arguments[4].([]byte)
			topic, _ = arguments[6].(string)
		} else if len(arguments) >= 6 {
			aggType, _ = arguments[1].(string)
			aggID, _ = arguments[2].(string)
			eventType, _ = arguments[3].(string)
			payload, _ = arguments[4].([]byte)
			topic, _ = arguments[5].(string)
		}

		if root.failOutboxEvent != "" && eventType == root.failOutboxEvent {
			return pgconn.CommandTag{}, root.failOutboxErr
		}

		if t.parent == nil || len(arguments) >= 7 {
			root.insertedOutbox = append(root.insertedOutbox, recordedOutboxRow{
				AggregateType: aggType,
				AggregateID:   aggID,
				EventType:     eventType,
				Payload:       payload,
				Topic:         topic,
			})
		}
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (t *fakeTx) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	return nil, nil
}

func (t *fakeTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	root := t.root()
	if strings.Contains(sql, "FOR UPDATE") {
		cp := *root.existing
		return &fakeDeliveryRow{delivery: &cp}
	}
	if strings.Contains(sql, "UPDATE deliveries") {
		updated := *root.existing
		if st, ok := args[1].(model.DeliveryStatus); ok {
			updated.Status = st
		}
		if est, ok := args[2].(*time.Time); ok {
			updated.EstimatedDelivery = est
		}
		if act, ok := args[3].(*time.Time); ok {
			updated.ActualDelivery = act
		}
		return &fakeDeliveryRow{delivery: &updated}
	}
	return &fakeDeliveryRow{err: pgx.ErrNoRows}
}

func (t *fakeTx) Conn() *pgx.Conn {
	return nil
}

func newTestDeliveryRepo(tx *fakeTx) *pgDeliveryRepository {
	repo := NewDeliveryRepositoryWithOutbox(nil, outbox.NewStore()).(*pgDeliveryRepository)
	repo.beginTx = func(ctx context.Context) (pgx.Tx, error) {
		return tx, nil
	}
	return repo
}

func sampleDelivery(status model.DeliveryStatus) *model.Delivery {
	now := time.Now().UTC()
	return &model.Delivery{
		ID:              uuid.NewString(),
		OrderID:         uuid.NewString(),
		DeliveryPartner: "DELHIVERY",
		TrackingNumber:  "GC-DEL-123456",
		Status:          status,
		CreatedAt:       now,
		UpdatedAt:       now,
	}
}

func TestUpdateStatusWithDetails_OrderEventFailuresRollbackTransaction(t *testing.T) {
	cases := []struct {
		name          string
		initialStatus model.DeliveryStatus
		targetStatus  model.DeliveryStatus
		eventType     string
	}{
		{
			name:          "DeliveryStatusChanged on OUT_FOR_DELIVERY",
			initialStatus: model.StatusInTransit,
			targetStatus:  model.StatusOutForDelivery,
			eventType:     events.EventTypeDeliveryStatusChanged,
		},
		{
			name:          "OrderShipped on PICKED_UP",
			initialStatus: model.StatusCreated,
			targetStatus:  model.StatusPickedUp,
			eventType:     events.EventTypeOrderShipped,
		},
		{
			name:          "OrderShipped on IN_TRANSIT",
			initialStatus: model.StatusPickedUp,
			targetStatus:  model.StatusInTransit,
			eventType:     events.EventTypeOrderShipped,
		},
		{
			name:          "OrderDelivered on DELIVERED",
			initialStatus: model.StatusOutForDelivery,
			targetStatus:  model.StatusDelivered,
			eventType:     events.EventTypeOrderDelivered,
		},
		{
			name:          "OrderCancelled on CANCELLED",
			initialStatus: model.StatusCreated,
			targetStatus:  model.StatusCancelled,
			eventType:     events.EventTypeOrderCancelled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name+"_EnvelopeCreationError", func(t *testing.T) {
			tx := &fakeTx{existing: sampleDelivery(tc.initialStatus)}
			repo := newTestDeliveryRepo(tx)
			sentinel := errors.New("envelope construction failure")
			repo.newEnvelope = func(eventType, source, aggregateID string, payload interface{}) (*events.EventEnvelope, error) {
				if eventType == tc.eventType {
					return nil, sentinel
				}
				return events.NewEventEnvelopeWithAggregate(eventType, source, aggregateID, payload)
			}

			updated, err := repo.UpdateStatusWithDetails(context.Background(), tx.existing.ID, tc.targetStatus, nil, nil, "Hub", "Test note")
			if err == nil {
				t.Fatalf("expected error when %s envelope creation fails, got nil", tc.eventType)
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected wrapped sentinel error %v, got %v", sentinel, err)
			}
			if appErr := appErrors.AsAppError(err); appErr == nil || appErr.Code != appErrors.CodeInternalError {
				t.Fatalf("expected internal AppError, got %v", appErr)
			}
			if updated != nil {
				t.Fatalf("expected nil delivery on error, got %+v", updated)
			}
			if tx.committed {
				t.Fatal("expected transaction NOT to be committed on envelope creation failure")
			}
			if !tx.rolledBack {
				t.Fatal("expected transaction to be rolled back on envelope creation failure")
			}
		})

		t.Run(tc.name+"_MarshalError", func(t *testing.T) {
			tx := &fakeTx{existing: sampleDelivery(tc.initialStatus)}
			repo := newTestDeliveryRepo(tx)
			sentinel := errors.New("marshal failure")
			repo.marshalEnvelope = func(env *events.EventEnvelope) ([]byte, error) {
				if env.EventType == tc.eventType {
					return nil, sentinel
				}
				return env.Marshal()
			}

			updated, err := repo.UpdateStatusWithDetails(context.Background(), tx.existing.ID, tc.targetStatus, nil, nil, "Hub", "Test note")
			if err == nil {
				t.Fatalf("expected error when %s marshaling fails, got nil", tc.eventType)
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected wrapped sentinel error %v, got %v", sentinel, err)
			}
			if updated != nil {
				t.Fatalf("expected nil delivery on error, got %+v", updated)
			}
			if tx.committed {
				t.Fatal("expected transaction NOT to be committed on marshal failure")
			}
			if !tx.rolledBack {
				t.Fatal("expected transaction to be rolled back on marshal failure")
			}
		})

		t.Run(tc.name+"_OutboxInsertError", func(t *testing.T) {
			sentinel := errors.New("outbox insert failure")
			tx := &fakeTx{
				existing:        sampleDelivery(tc.initialStatus),
				failOutboxEvent: tc.eventType,
				failOutboxErr:   sentinel,
			}
			repo := newTestDeliveryRepo(tx)

			updated, err := repo.UpdateStatusWithDetails(context.Background(), tx.existing.ID, tc.targetStatus, nil, nil, "Hub", "Test note")
			if err == nil {
				t.Fatalf("expected error when %s outbox insert fails, got nil", tc.eventType)
			}
			if !errors.Is(err, sentinel) {
				t.Fatalf("expected wrapped sentinel error %v, got %v", sentinel, err)
			}
			if updated != nil {
				t.Fatalf("expected nil delivery on error, got %+v", updated)
			}
			if tx.committed {
				t.Fatal("expected transaction NOT to be committed on outbox insert failure")
			}
			if !tx.rolledBack {
				t.Fatal("expected transaction to be rolled back on outbox insert failure")
			}
		})
	}
}

func TestUpdateStatusWithDetails_OrderEventsCommittedOnSuccess(t *testing.T) {
	cases := []struct {
		name          string
		initialStatus model.DeliveryStatus
		targetStatus  model.DeliveryStatus
		expectedEvent string
		expectedTopic string
	}{
		{
			name:          "PICKED_UP emits OrderShipped",
			initialStatus: model.StatusCreated,
			targetStatus:  model.StatusPickedUp,
			expectedEvent: events.EventTypeOrderShipped,
			expectedTopic: events.TopicOrderShipped,
		},
		{
			name:          "IN_TRANSIT emits OrderShipped",
			initialStatus: model.StatusPickedUp,
			targetStatus:  model.StatusInTransit,
			expectedEvent: events.EventTypeOrderShipped,
			expectedTopic: events.TopicOrderShipped,
		},
		{
			name:          "DELIVERED emits OrderDelivered",
			initialStatus: model.StatusOutForDelivery,
			targetStatus:  model.StatusDelivered,
			expectedEvent: events.EventTypeOrderDelivered,
			expectedTopic: events.TopicOrderDelivered,
		},
		{
			name:          "CANCELLED emits OrderCancelled",
			initialStatus: model.StatusCreated,
			targetStatus:  model.StatusCancelled,
			expectedEvent: events.EventTypeOrderCancelled,
			expectedTopic: events.TopicOrderCancelled,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tx := &fakeTx{existing: sampleDelivery(tc.initialStatus)}
			repo := newTestDeliveryRepo(tx)

			updated, err := repo.UpdateStatusWithDetails(context.Background(), tx.existing.ID, tc.targetStatus, nil, nil, "Hub", "Customer requested cancel")
			if err != nil {
				t.Fatalf("expected no error, got %v", err)
			}
			if updated == nil || updated.Status != tc.targetStatus {
				t.Fatalf("expected updated delivery with status %s, got %+v", tc.targetStatus, updated)
			}
			if !tx.committed {
				t.Fatal("expected transaction to be committed")
			}
			if tx.rolledBack {
				t.Fatal("expected transaction NOT to be rolled back")
			}

			var found bool
			for _, row := range tx.insertedOutbox {
				if row.EventType == tc.expectedEvent {
					found = true
					if row.Topic != tc.expectedTopic {
						t.Errorf("expected topic %s, got %s", tc.expectedTopic, row.Topic)
					}
					if row.AggregateType != "order" {
						t.Errorf("expected aggregate_type 'order', got %s", row.AggregateType)
					}
					if row.AggregateID != tx.existing.OrderID {
						t.Errorf("expected aggregate_id %s, got %s", tx.existing.OrderID, row.AggregateID)
					}
					var env events.EventEnvelope
					if err := json.Unmarshal(row.Payload, &env); err != nil {
						t.Fatalf("failed to unmarshal outbox payload envelope: %v", err)
					}
					if env.EventType != tc.expectedEvent {
						t.Errorf("expected envelope event_type %s, got %s", tc.expectedEvent, env.EventType)
					}
				}
			}
			if !found {
				t.Fatalf("expected outbox event %s to be inserted, got %+v", tc.expectedEvent, tx.insertedOutbox)
			}
		})
	}
}

