-- Add the topic column so the generic outbox publisher knows which Kafka topic
-- to route each event to without any service-specific publisher logic.
-- DEFAULT '' prevents NOT NULL violations on any pre-existing PENDING rows.
ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';

-- Partial index to accelerate publisher polling: only PENDING rows matter.
CREATE INDEX IF NOT EXISTS idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING';
