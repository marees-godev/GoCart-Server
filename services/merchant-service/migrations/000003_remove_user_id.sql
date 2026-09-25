ALTER TABLE merchants DROP COLUMN IF EXISTS user_id;
DROP INDEX IF EXISTS idx_merchants_user_id;
ALTER TABLE merchants ALTER COLUMN business_name SET DEFAULT '';
