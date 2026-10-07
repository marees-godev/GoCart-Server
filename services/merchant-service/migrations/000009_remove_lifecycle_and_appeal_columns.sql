ALTER TABLE merchant_lifecycle_audit DROP COLUMN IF EXISTS error_message;
ALTER TABLE merchant_lifecycle_audit DROP COLUMN IF EXISTS request_id;

ALTER TABLE merchant_appeals DROP COLUMN IF EXISTS admin_comment;
ALTER TABLE merchant_appeals DROP COLUMN IF EXISTS reviewed_at;
