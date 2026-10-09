ALTER TABLE merchant_lifecycle_audit DROP COLUMN IF EXISTS error_message;
ALTER TABLE merchant_lifecycle_audit DROP COLUMN IF EXISTS request_id;

ALTER TABLE merchant_appeals ADD COLUMN IF NOT EXISTS admin_comment TEXT;
ALTER TABLE merchant_appeals ADD COLUMN IF NOT EXISTS reviewed_at TIMESTAMPTZ;
