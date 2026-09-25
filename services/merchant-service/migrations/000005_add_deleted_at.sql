ALTER TABLE merchants ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ DEFAULT NULL;
CREATE INDEX IF NOT EXISTS idx_merchants_deleted_at ON merchants(deleted_at);
