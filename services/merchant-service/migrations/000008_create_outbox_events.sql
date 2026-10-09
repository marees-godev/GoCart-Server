-- 000008_create_outbox_events.sql
-- Create outbox_status ENUM and configure outbox_events table for transactional outbox pattern.

DO $$ BEGIN
    CREATE TYPE outbox_status AS ENUM ('PENDING', 'PUBLISHED', 'FAILED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL DEFAULT 'merchant',
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    topic VARCHAR(255) NOT NULL DEFAULT '',
    status outbox_status NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

-- Ensure newly added columns exist if table was partially created in earlier migration
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS aggregate_type VARCHAR(100) NOT NULL DEFAULT 'merchant';
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS headers JSONB NOT NULL DEFAULT '{}'::jsonb;
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW();
ALTER TABLE outbox_events ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

-- Ensure status column uses explicit outbox_status ENUM type
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

-- Polling performance indexes
CREATE INDEX IF NOT EXISTS idx_merchant_outbox_status_created ON outbox_events(status, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_merchant_outbox_pending_created ON outbox_events(created_at ASC) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_merchant_outbox_aggregate ON outbox_events(aggregate_id);
