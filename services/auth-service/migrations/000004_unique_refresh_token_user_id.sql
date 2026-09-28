CREATE TABLE IF NOT EXISTS refresh_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES auth_credentials(user_id) ON DELETE CASCADE,
    token_hash VARCHAR(255) NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked BOOLEAN NOT NULL DEFAULT FALSE,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Remove duplicate refresh tokens keeping only the newest per user_id before adding unique index
DELETE FROM refresh_tokens a USING refresh_tokens b
WHERE a.user_id = b.user_id AND a.created_at < b.created_at;

ALTER TABLE refresh_tokens DROP CONSTRAINT IF EXISTS refresh_tokens_user_id_key;
DROP INDEX IF EXISTS idx_refresh_tokens_user_id;
CREATE UNIQUE INDEX IF NOT EXISTS idx_refresh_tokens_user_id_unique ON refresh_tokens (user_id);
