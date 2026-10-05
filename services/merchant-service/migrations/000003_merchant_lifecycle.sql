ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'ACTIVE';
ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'INACTIVE';
ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'PENDING_REVIEW';
ALTER TYPE merchant_status ADD VALUE IF NOT EXISTS 'TERMINATED';

ALTER TABLE merchants ADD COLUMN IF NOT EXISTS version INT NOT NULL DEFAULT 1;

DO $$ BEGIN
    CREATE TYPE merchant_lifecycle_action AS ENUM ('ACTIVATE', 'SUSPEND', 'REACTIVATE', 'APPROVE', 'REJECT', 'UPDATE_STATUS');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'APPROVE';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'REJECT';
ALTER TYPE merchant_lifecycle_action ADD VALUE IF NOT EXISTS 'UPDATE_STATUS';

DO $$ BEGIN
    CREATE TYPE merchant_audit_status AS ENUM ('SUCCESS', 'FAILED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DROP TABLE IF EXISTS merchant_status_audit;

CREATE TABLE IF NOT EXISTS merchant_lifecycle_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    admin_id VARCHAR(100) NOT NULL,
    action merchant_lifecycle_action NOT NULL,
    previous_status VARCHAR(50) NOT NULL,
    new_status VARCHAR(50) NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    status merchant_audit_status NOT NULL DEFAULT 'SUCCESS',
    error_message TEXT NOT NULL DEFAULT '',
    request_id VARCHAR(100) NOT NULL DEFAULT '',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_lifecycle_audit_merchant_id ON merchant_lifecycle_audit(merchant_id);
CREATE INDEX IF NOT EXISTS idx_merchant_lifecycle_audit_created_at ON merchant_lifecycle_audit(created_at);
CREATE INDEX IF NOT EXISTS idx_merchant_lifecycle_audit_admin_id ON merchant_lifecycle_audit(admin_id);
