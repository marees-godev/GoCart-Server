ALTER TABLE auth_credentials DROP CONSTRAINT IF EXISTS auth_credentials_email_key;
DROP INDEX IF EXISTS idx_auth_credentials_email;
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_credentials_email_role ON auth_credentials (LOWER(email), role);
