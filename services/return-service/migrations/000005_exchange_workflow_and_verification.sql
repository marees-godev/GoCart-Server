DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'exchange_status') THEN
        CREATE TYPE exchange_status AS ENUM (
            'OFFER_CREATED',
            'APPLIED_AT_CHECKOUT',
            'PICKUP_SCHEDULED',
            'PICKED_UP',
            'VERIFIED',
            'REVALUATED',
            'COMPLETED',
            'CANCELLED'
        );
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'exchange_condition_rating') THEN
        CREATE TYPE exchange_condition_rating AS ENUM (
            'EXCELLENT',
            'GOOD',
            'AVERAGE',
            'POOR',
            'DAMAGED'
        );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS exchanges (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    order_item_id UUID NOT NULL,
    user_id UUID NOT NULL,
    new_product_id UUID NOT NULL,
    old_brand VARCHAR(100) NOT NULL,
    old_model VARCHAR(100) NOT NULL,
    old_variant VARCHAR(100),
    serial_or_imei VARCHAR(100),
    base_exchange_value DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (base_exchange_value >= 0),
    estimated_discount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (estimated_discount >= 0),
    final_discount DECIMAL(12,2) DEFAULT 0.00 CHECK (final_discount >= 0),
    price_difference DECIMAL(12,2) DEFAULT 0.00,
    status exchange_status NOT NULL DEFAULT 'OFFER_CREATED',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS exchange_questionnaire_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange_id UUID NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    question_key VARCHAR(100) NOT NULL,
    answer_value VARCHAR(255) NOT NULL,
    deduction_amount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (deduction_amount >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS exchange_verifications (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    exchange_id UUID NOT NULL REFERENCES exchanges(id) ON DELETE CASCADE,
    verified_by UUID NOT NULL,
    condition_rating exchange_condition_rating NOT NULL DEFAULT 'GOOD',
    is_approved BOOLEAN NOT NULL DEFAULT FALSE,
    screen_intact BOOLEAN NOT NULL DEFAULT TRUE,
    body_intact BOOLEAN NOT NULL DEFAULT TRUE,
    functional BOOLEAN NOT NULL DEFAULT TRUE,
    serial_matches BOOLEAN NOT NULL DEFAULT TRUE,
    adjusted_discount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (adjusted_discount >= 0),
    notes TEXT,
    verified_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_exchange_verifications_exchange_id UNIQUE (exchange_id)
);

CREATE TABLE IF NOT EXISTS exchange_verification_images (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    verification_id UUID NOT NULL REFERENCES exchange_verifications(id) ON DELETE CASCADE,
    image_url TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_exchanges_order_id ON exchanges(order_id);
CREATE INDEX IF NOT EXISTS idx_exchanges_user_id ON exchanges(user_id);
CREATE INDEX IF NOT EXISTS idx_exchanges_status ON exchanges(status);
CREATE INDEX IF NOT EXISTS idx_exchange_qa_exchange_id ON exchange_questionnaire_answers(exchange_id);
CREATE INDEX IF NOT EXISTS idx_exchange_verifications_exchange_id ON exchange_verifications(exchange_id);
CREATE INDEX IF NOT EXISTS idx_exchange_verifications_verified_by ON exchange_verifications(verified_by);
CREATE INDEX IF NOT EXISTS idx_exchange_verification_images_ver_id ON exchange_verification_images(verification_id);
