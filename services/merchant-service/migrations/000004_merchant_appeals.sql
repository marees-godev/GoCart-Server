DO $$ BEGIN
    CREATE TYPE merchant_lifecycle_status AS ENUM ('ACTIVE', 'INACTIVE', 'PENDING_REVIEW', 'SUSPENDED', 'TERMINATED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS lifecycle_status merchant_lifecycle_status DEFAULT NULL;

DO $$ BEGIN
    CREATE TYPE merchant_appeal_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS merchant_appeals (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    reason TEXT NOT NULL,
    status merchant_appeal_status NOT NULL DEFAULT 'PENDING',
    admin_comment TEXT,
    reviewed_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_appeals_merchant_id ON merchant_appeals(merchant_id);
CREATE INDEX IF NOT EXISTS idx_merchant_appeals_status ON merchant_appeals(status);
