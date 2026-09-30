CREATE TABLE IF NOT EXISTS merchant_status_audit (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL REFERENCES merchants(id) ON DELETE CASCADE,
    from_status merchant_status NOT NULL,
    to_status merchant_status NOT NULL,
    reason TEXT NOT NULL DEFAULT '',
    updated_by VARCHAR(100) NOT NULL DEFAULT 'ADMIN',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_merchant_status_audit_merchant_id ON merchant_status_audit(merchant_id);
CREATE INDEX IF NOT EXISTS idx_merchant_status_audit_created_at ON merchant_status_audit(created_at);
