CREATE TYPE flag_category AS ENUM (
    'SPAM',
    'INAPPROPRIATE_CONTENT',
    'FAKE_REVIEW',
    'IRRELEVANT',
    'OTHER'
);

CREATE TABLE IF NOT EXISTS rating_flags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    rating_id UUID NOT NULL REFERENCES ratings(id) ON DELETE CASCADE,
    user_id UUID NOT NULL,
    category flag_category NOT NULL DEFAULT 'SPAM',
    reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(rating_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_rating_flags_rating_id ON rating_flags(rating_id);
CREATE INDEX IF NOT EXISTS idx_rating_flags_user_id ON rating_flags(user_id);