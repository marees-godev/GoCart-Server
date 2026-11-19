ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS label VARCHAR(50);
ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS full_name VARCHAR(100);
ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS phone_number VARCHAR(30);
ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS email_address VARCHAR(255);
ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS address_line VARCHAR(255);
ALTER TABLE user_addresses ADD COLUMN IF NOT EXISTS country VARCHAR(100);
ALTER TABLE user_addresses ALTER COLUMN country_id DROP NOT NULL;
CREATE INDEX IF NOT EXISTS idx_user_addresses_user_default ON user_addresses(user_id, is_default);
