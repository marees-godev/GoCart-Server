DO $$ BEGIN
    CREATE TYPE outbox_status AS ENUM (
        'PENDING',
        'PROCESSING',
        'PUBLISHED',
        'FAILED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS headers JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'outbox_events' AND column_name = 'status' AND data_type = 'character varying'
    ) THEN
        ALTER TABLE outbox_events ALTER COLUMN status DROP DEFAULT;
        ALTER TABLE outbox_events ALTER COLUMN status TYPE outbox_status USING status::outbox_status;
        ALTER TABLE outbox_events ALTER COLUMN status SET DEFAULT 'PENDING'::outbox_status;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_delivery_outbox_status_created ON outbox_events(status, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_created ON outbox_events(created_at ASC) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_delivery_outbox_aggregate ON outbox_events(aggregate_id);

ALTER TABLE deliveries DROP COLUMN IF EXISTS delivery_partner_id;
DROP TABLE IF EXISTS delivery_partners CASCADE;