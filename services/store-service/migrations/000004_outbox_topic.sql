-- Add topic column to outbox_events to support generic outbox event routing.
ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';

-- Partial index to optimize publisher queries for pending outbox events.
CREATE INDEX IF NOT EXISTS idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING';

-- Ensure IFSC code
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'store_bank_accounts' AND column_name = 'routing_number'
    ) THEN
        ALTER TABLE store_bank_accounts RENAME COLUMN routing_number TO ifsc_code;
    END IF;
END $$;

ALTER TABLE store_bank_accounts ADD COLUMN IF NOT EXISTS ifsc_code VARCHAR(100);
ALTER TABLE stores ADD COLUMN IF NOT EXISTS is_published BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE stores ADD COLUMN IF NOT EXISTS gstin VARCHAR(15);
