-- 000006_google_oauth.sql
-- Add Google OAuth provider support, oauth_accounts table, and allow null password_hash for OAuth users

DO $$ BEGIN
    CREATE TYPE auth_provider_type AS ENUM ('LOCAL', 'GOOGLE');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS oauth_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_credentials(user_id) ON DELETE CASCADE,
    provider auth_provider_type NOT NULL,
    provider_user_id VARCHAR(255) NOT NULL,
    email VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_oauth_provider_user UNIQUE (provider, provider_user_id)
);

CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user_id ON oauth_accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_oauth_accounts_email ON oauth_accounts(LOWER(email));

ALTER TABLE auth_credentials ALTER COLUMN password_hash DROP NOT NULL;

