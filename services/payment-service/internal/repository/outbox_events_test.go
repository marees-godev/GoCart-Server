package repository_test

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/joho/godotenv"
	"github.com/marees-godev/GoCart-Server/contracts/events"
	"github.com/marees-godev/GoCart-Server/pkg/outbox"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/model"
	"github.com/marees-godev/GoCart-Server/services/payment-service/internal/repository"
)

func TestOutboxPaymentSuccessfulEventPayloadIntegrity(t *testing.T) {
	paymentID := uuid.New().String()
	orderID := uuid.New().String()
	userID := uuid.New().String()
	paidAt := time.Now().UTC().Truncate(time.Millisecond)

	domainEvent := events.PaymentSuccessfulEvent{
		PaymentID:     paymentID,
		OrderID:       orderID,
		UserID:        userID,
		Amount:        99.99,
		Currency:      "INR",
		PaymentMethod: "UPI",
		PaidAt:        paidAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentSuccessful, "payment-service", paymentID, domainEvent)
	if err != nil {
		t.Fatalf("failed to create event envelope: %v", err)
	}

	if env.EventType != events.EventTypePaymentSuccessful {
		t.Errorf("expected event type %s, got %s", events.EventTypePaymentSuccessful, env.EventType)
	}
	if env.AggregateID != paymentID {
		t.Errorf("expected aggregate ID %s, got %s", paymentID, env.AggregateID)
	}
	if env.Source != "payment-service" {
		t.Errorf("expected source payment-service, got %s", env.Source)
	}

	payloadBytes, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal event envelope: %v", err)
	}

	// Verify valid JSON
	var rawJSON map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &rawJSON); err != nil {
		t.Fatalf("payloadBytes is not valid JSON: %v", err)
	}

	// Verify publisher can unmarshal and extract domain data
	unmarshaledEnv, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal envelope with events.UnmarshalEnvelope: %v", err)
	}

	var extracted events.PaymentSuccessfulEvent
	if err := unmarshaledEnv.UnmarshalData(&extracted); err != nil {
		t.Fatalf("failed to unmarshal domain data: %v", err)
	}

	if extracted.PaymentID != paymentID || extracted.OrderID != orderID || extracted.Amount != 99.99 || extracted.Currency != "INR" {
		t.Errorf("extracted domain event mismatch: %+v", extracted)
	}

	// Verify topic routing matches expectations
	if events.TopicPaymentSuccessful != "gocart.payment.payment-successful" {
		t.Errorf("unexpected topic for PaymentSuccessful: %s", events.TopicPaymentSuccessful)
	}
}

func TestOutboxPaymentFailedEventPayloadIntegrity(t *testing.T) {
	paymentID := uuid.New().String()
	orderID := uuid.New().String()
	userID := uuid.New().String()
	failedAt := time.Now().UTC().Truncate(time.Millisecond)

	domainEvent := events.PaymentFailedEvent{
		PaymentID: paymentID,
		OrderID:   orderID,
		UserID:    userID,
		Amount:    150.00,
		Reason:    "insufficient funds",
		FailedAt:  failedAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentFailed, "payment-service", paymentID, domainEvent)
	if err != nil {
		t.Fatalf("failed to create event envelope: %v", err)
	}

	payloadBytes, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal event envelope: %v", err)
	}

	unmarshaledEnv, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	var extracted events.PaymentFailedEvent
	if err := unmarshaledEnv.UnmarshalData(&extracted); err != nil {
		t.Fatalf("failed to unmarshal failed domain data: %v", err)
	}

	if extracted.PaymentID != paymentID || extracted.Reason != "insufficient funds" || extracted.Amount != 150.00 {
		t.Errorf("extracted failed event mismatch: %+v", extracted)
	}

	if events.TopicPaymentFailed != "gocart.payment.payment-failed" {
		t.Errorf("unexpected topic for PaymentFailed: %s", events.TopicPaymentFailed)
	}
}

func TestOutboxPaymentPendingEventPayloadIntegrity(t *testing.T) {
	paymentID := uuid.New().String()
	orderID := uuid.New().String()
	userID := uuid.New().String()
	pendingAt := time.Now().UTC().Truncate(time.Millisecond)

	domainEvent := events.PaymentPendingEvent{
		PaymentID:            paymentID,
		OrderID:              orderID,
		UserID:               userID,
		Amount:               50.00,
		Currency:             "INR",
		GatewayTransactionID: "pay_xyz123",
		PendingAt:            pendingAt,
	}

	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentPending, "payment-service", paymentID, domainEvent)
	if err != nil {
		t.Fatalf("failed to create event envelope: %v", err)
	}

	payloadBytes, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal event envelope: %v", err)
	}

	unmarshaledEnv, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal envelope: %v", err)
	}

	var extracted events.PaymentPendingEvent
	if err := unmarshaledEnv.UnmarshalData(&extracted); err != nil {
		t.Fatalf("failed to unmarshal pending domain data: %v", err)
	}

	if extracted.PaymentID != paymentID || extracted.GatewayTransactionID != "pay_xyz123" {
		t.Errorf("extracted pending event mismatch: %+v", extracted)
	}

	if events.TopicPaymentPending != "gocart.payment.payment-pending" {
		t.Errorf("unexpected topic for PaymentPending: %s", events.TopicPaymentPending)
	}
}

func TestOutboxPublisherCompatibilityWithPaymentEvents(t *testing.T) {
	paymentID := uuid.New().String()
	env, err := events.NewEventEnvelopeWithAggregate(events.EventTypePaymentSuccessful, "payment-service", paymentID, events.PaymentSuccessfulEvent{
		PaymentID: paymentID,
		Amount:    200.0,
	})
	if err != nil {
		t.Fatalf("failed to create envelope: %v", err)
	}

	payload, err := env.Marshal()
	if err != nil {
		t.Fatalf("failed to marshal payload: %v", err)
	}

	// Simulate what outbox.Store / outbox.Publisher expects when polling
	outboxEvt := &outbox.Event{
		AggregateType: "payment",
		AggregateID:   paymentID,
		EventType:     events.EventTypePaymentSuccessful,
		Payload:       payload,
		Topic:         events.TopicPaymentSuccessful,
		Status:        outbox.StatusPending,
		RetryCount:    0,
		CreatedAt:     time.Now().UTC(),
	}

	if outboxEvt.Status != outbox.StatusPending {
		t.Errorf("expected outbox status PENDING, got %s", outboxEvt.Status)
	}
	if outboxEvt.AggregateType != "payment" {
		t.Errorf("expected aggregate type payment, got %s", outboxEvt.AggregateType)
	}
	if outboxEvt.Topic != events.TopicPaymentSuccessful {
		t.Errorf("expected topic %s, got %s", events.TopicPaymentSuccessful, outboxEvt.Topic)
	}

	// Verify unmarshaling in publisher process loop
	unmarshaled, err := events.UnmarshalEnvelope(outboxEvt.Payload)
	if err != nil {
		t.Fatalf("outbox publisher unmarshaling failed: %v", err)
	}
	if unmarshaled.AggregateID != paymentID {
		t.Errorf("expected aggregate ID %s, got %s", paymentID, unmarshaled.AggregateID)
	}
}

func TestTerminalStateTransitionsPreventRogueOutboxEvents(t *testing.T) {
	terminalStates := []model.PaymentStatus{
		model.PaymentStatusSuccess,
		model.PaymentStatusFailed,
		model.PaymentStatusCancelled,
		model.PaymentStatusRefunded,
	}

	for _, ts := range terminalStates {
		// Terminal states cannot transition to PENDING
		if model.CanTransitionPayment(ts, model.PaymentStatusPending) {
			t.Errorf("illegal transition allowed: %s -> PENDING", ts)
		}
		// FAILED and CANCELLED cannot transition to SUCCESS
		if ts == model.PaymentStatusFailed && model.CanTransitionPayment(ts, model.PaymentStatusSuccess) {
			t.Errorf("illegal transition allowed: FAILED -> SUCCESS")
		}
		if ts == model.PaymentStatusCancelled && model.CanTransitionPayment(ts, model.PaymentStatusSuccess) {
			t.Errorf("illegal transition allowed: CANCELLED -> SUCCESS")
		}
	}
}

func TestLiveDatabaseOutboxEventsAndPaymentStatusTransitions(t *testing.T) {
	_ = godotenv.Load("../../.env")
	_ = godotenv.Load(".env")
	dbURL := os.Getenv("PAYMENT_SERVICE_DATABASE_URL")
	if dbURL == "" {
		t.Skip("Skipping live database test: PAYMENT_SERVICE_DATABASE_URL not configured")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, dbURL)
	if err != nil {
		t.Fatalf("failed to connect to database pool: %v", err)
	}
	defer pool.Close()

	repo := repository.NewPaymentRepository(pool)

	// Test 1: Successful payment flow
	payID := uuid.New().String()
	orderID := uuid.New().String()
	userID := uuid.New().String()
	idempKey := fmt.Sprintf("idemp_test_%s", uuid.New().String()[:8])
	gtxID := fmt.Sprintf("order_rzp_%s", uuid.New().String()[:8])

	pmt := &model.Payment{
		ID:                   payID,
		OrderID:              orderID,
		UserID:               userID,
		PaymentMethod:        "UPI",
		Amount:               1299.00,
		Currency:             "INR",
		Status:               model.PaymentStatusPending,
		TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
		GatewayTransactionID: gtxID,
		IdempotencyKey:       idempKey,
	}

	if err := repo.CreatePayment(ctx, pmt); err != nil {
		t.Fatalf("CreatePayment failed: %v", err)
	}

	var initialStatus string
	if err := pool.QueryRow(ctx, "SELECT status FROM payments WHERE id = $1", payID).Scan(&initialStatus); err != nil {
		t.Fatalf("failed to query payment status: %v", err)
	}
	if initialStatus != "PENDING" {
		t.Errorf("expected initial status PENDING, got %s", initialStatus)
	}

	// Update to SUCCESS
	completedGtxID := fmt.Sprintf("pay_rzp_%s", uuid.New().String()[:8])
	updatedPmt, err := repo.UpdatePaymentStatus(ctx, payID, model.PaymentStatusSuccess, completedGtxID, "", "UPI")
	if err != nil {
		t.Fatalf("UpdatePaymentStatus to SUCCESS failed: %v", err)
	}
	if updatedPmt.Status != model.PaymentStatusSuccess {
		t.Errorf("expected updated payment model status SUCCESS, got %s", updatedPmt.Status)
	}

	// Verify payments table in DB
	var dbStatus, dbGtxID string
	if err := pool.QueryRow(ctx, "SELECT status, gateway_transaction_id FROM payments WHERE id = $1", payID).Scan(&dbStatus, &dbGtxID); err != nil {
		t.Fatalf("failed to query updated payment from DB: %v", err)
	}
	if dbStatus != "SUCCESS" {
		t.Errorf("expected payments table status SUCCESS, got %s", dbStatus)
	}
	if dbGtxID != completedGtxID {
		t.Errorf("expected gateway_transaction_id %s, got %s", completedGtxID, dbGtxID)
	}

	// Verify outbox_events table in DB
	var outboxID, eventType, topic, outboxStatus string
	var payloadBytes []byte
	err = pool.QueryRow(ctx, `
		SELECT id, event_type, topic, status, payload 
		FROM outbox_events 
		WHERE aggregate_id = $1 AND event_type = $2
	`, payID, events.EventTypePaymentSuccessful).Scan(&outboxID, &eventType, &topic, &outboxStatus, &payloadBytes)
	if err != nil {
		t.Fatalf("outbox event for PaymentSuccessful not found in DB: %v", err)
	}

	if eventType != events.EventTypePaymentSuccessful {
		t.Errorf("expected outbox event_type %s, got %s", events.EventTypePaymentSuccessful, eventType)
	}
	if topic != events.TopicPaymentSuccessful {
		t.Errorf("expected outbox topic %s, got %s", events.TopicPaymentSuccessful, topic)
	}
	if outboxStatus != "PENDING" && outboxStatus != "PUBLISHED" && outboxStatus != "FAILED" {
		t.Errorf("unexpected outbox status %s", outboxStatus)
	}

	env, err := events.UnmarshalEnvelope(payloadBytes)
	if err != nil {
		t.Fatalf("failed to unmarshal stored outbox payload: %v", err)
	}
	if env.EventType != events.EventTypePaymentSuccessful || env.AggregateID != payID {
		t.Errorf("mismatched envelope metadata: EventType=%s, AggregateID=%s", env.EventType, env.AggregateID)
	}

	// Verify duplicate transition does NOT duplicate outbox events
	_, err = repo.UpdatePaymentStatus(ctx, payID, model.PaymentStatusSuccess, completedGtxID, "")
	if err != nil {
		t.Fatalf("duplicate UpdatePaymentStatus failed: %v", err)
	}

	var count int
	if err := pool.QueryRow(ctx, "SELECT COUNT(*) FROM outbox_events WHERE aggregate_id = $1 AND event_type = $2", payID, events.EventTypePaymentSuccessful).Scan(&count); err != nil {
		t.Fatalf("failed to count outbox events: %v", err)
	}
	if count != 1 {
		t.Errorf("expected exactly 1 outbox event for PaymentSuccessful, found %d", count)
	}

	// Test 2: Failed payment flow
	failPayID := uuid.New().String()
	failPmt := &model.Payment{
		ID:                   failPayID,
		OrderID:              uuid.New().String(),
		UserID:               userID,
		PaymentMethod:        "DEBIT_CARD",
		Amount:               500.00,
		Currency:             "INR",
		Status:               model.PaymentStatusPending,
		TransactionID:        fmt.Sprintf("tx_%s", uuid.New().String()),
		GatewayTransactionID: fmt.Sprintf("order_rzp_%s", uuid.New().String()[:8]),
		IdempotencyKey:       fmt.Sprintf("idemp_fail_%s", uuid.New().String()[:8]),
	}
	if err := repo.CreatePayment(ctx, failPmt); err != nil {
		t.Fatalf("CreatePayment for fail test failed: %v", err)
	}

	failReason := "Payment failed due to card decline"
	updatedFailPmt, err := repo.UpdatePaymentStatus(ctx, failPayID, model.PaymentStatusFailed, "", failReason)
	if err != nil {
		t.Fatalf("UpdatePaymentStatus to FAILED failed: %v", err)
	}
	if updatedFailPmt.Status != model.PaymentStatusFailed {
		t.Errorf("expected status FAILED, got %s", updatedFailPmt.Status)
	}

	var failDbStatus, failDbReason string
	if err := pool.QueryRow(ctx, "SELECT status, failure_reason FROM payments WHERE id = $1", failPayID).Scan(&failDbStatus, &failDbReason); err != nil {
		t.Fatalf("failed to query failed payment from DB: %v", err)
	}
	if failDbStatus != "FAILED" || failDbReason != failReason {
		t.Errorf("expected DB status FAILED and reason %q, got status %s, reason %q", failReason, failDbStatus, failDbReason)
	}

	var failEventType, failTopic string
	err = pool.QueryRow(ctx, `
		SELECT event_type, topic 
		FROM outbox_events 
		WHERE aggregate_id = $1 AND event_type = $2
	`, failPayID, events.EventTypePaymentFailed).Scan(&failEventType, &failTopic)
	if err != nil {
		t.Fatalf("outbox event for PaymentFailed not found in DB: %v", err)
	}
	if failEventType != events.EventTypePaymentFailed {
		t.Errorf("expected outbox event_type %s, got %s", events.EventTypePaymentFailed, failEventType)
	}
	if failTopic != events.TopicPaymentFailed {
		t.Errorf("expected outbox topic %s, got %s", events.TopicPaymentFailed, failTopic)
	}
}
