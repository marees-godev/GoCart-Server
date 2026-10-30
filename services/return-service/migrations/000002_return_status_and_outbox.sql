DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'return_status') THEN
        CREATE TYPE return_status AS ENUM (
            'RETURN_REQUESTED',
            'APPROVED',
            'REJECTED',
            'ITEM_RECEIVED',
            'REFUND_INITIATED',
            'COMPLETED',
            'CANCELLED'
        );
    END IF;
END $$;

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'returns' AND column_name = 'status' AND udt_name = 'varchar'
    ) THEN
        ALTER TABLE returns ALTER COLUMN status DROP DEFAULT;
        ALTER TABLE returns ALTER COLUMN status TYPE return_status USING (status::return_status);
        ALTER TABLE returns ALTER COLUMN status SET DEFAULT 'RETURN_REQUESTED'::return_status;
    END IF;
END $$;

CREATE UNIQUE INDEX IF NOT EXISTS idx_returns_order_item_id_unique ON returns(order_item_id);

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING';
