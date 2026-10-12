-- 000003_outbox_topic.sql
-- Add topic column and explicit outbox_status enum for product outbox events.

DO $$ BEGIN
    CREATE TYPE outbox_status AS ENUM ('PENDING', 'PUBLISHED', 'FAILED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';

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

CREATE INDEX IF NOT EXISTS idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING';

ALTER TABLE products ALTER COLUMN status SET DEFAULT 'DRAFT';

