DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'return_type') THEN
        CREATE TYPE return_type AS ENUM (
            'RETURN',
            'REPLACEMENT'
        );
    END IF;
END $$;

ALTER TYPE return_status ADD VALUE IF NOT EXISTS 'REPLACEMENT_REQUESTED';
ALTER TYPE return_status ADD VALUE IF NOT EXISTS 'VERIFIED';
ALTER TYPE return_status ADD VALUE IF NOT EXISTS 'REPLACEMENT_INITIATED';

ALTER TABLE returns
    ADD COLUMN IF NOT EXISTS type return_type NOT NULL DEFAULT 'RETURN';

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'verification_condition') THEN
        CREATE TYPE verification_condition AS ENUM (
            'GOOD',
            'DEFECTIVE',
            'DAMAGED',
            'WRONG_ITEM',
            'OPEN_BOX'
        );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS return_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    return_id UUID NOT NULL REFERENCES returns(id) ON DELETE CASCADE,
    verified_by UUID NOT NULL,
    condition verification_condition NOT NULL DEFAULT 'GOOD',
    is_approved BOOLEAN NOT NULL DEFAULT FALSE,
    notes TEXT,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_return_verifications_return_id UNIQUE (return_id)
);

CREATE TABLE IF NOT EXISTS return_verification_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    verification_id UUID NOT NULL REFERENCES return_verifications(id) ON DELETE CASCADE,
    image_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_returns_type ON returns(type);
CREATE INDEX IF NOT EXISTS idx_return_verifications_return_id ON return_verifications(return_id);
CREATE INDEX IF NOT EXISTS idx_return_verifications_verified_by ON return_verifications(verified_by);
CREATE INDEX IF NOT EXISTS idx_return_verification_images_verification_id ON return_verification_images(verification_id);
