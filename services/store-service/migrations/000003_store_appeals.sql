DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_appeal_status') THEN
        CREATE TYPE store_appeal_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS store_appeals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    merchant_id UUID NOT NULL,
    reason TEXT NOT NULL,
    status store_appeal_status NOT NULL DEFAULT 'PENDING',
    admin_comment TEXT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_store_appeals_store_id ON store_appeals(store_id);
CREATE INDEX IF NOT EXISTS idx_store_appeals_merchant_id ON store_appeals(merchant_id);
CREATE INDEX IF NOT EXISTS idx_store_appeals_status ON store_appeals(status);
