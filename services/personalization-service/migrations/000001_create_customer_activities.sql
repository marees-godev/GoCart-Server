-- 1. Explicit PostgreSQL ENUM for activity event types
DO $$ BEGIN
    CREATE TYPE activity_event_type AS ENUM (
        'PRODUCT_VIEWED',
        'PRODUCT_SEARCHED',
        'CATEGORY_VIEWED',
        'PRODUCT_ADDED_TO_CART',
        'PRODUCT_REMOVED_FROM_CART',
        'PRODUCT_WISHLISTED',
        'PRODUCT_PURCHASED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- 2. Customer activities table
CREATE TABLE IF NOT EXISTS customer_activities (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NULL,
    session_id VARCHAR(128) NULL,
    event_type activity_event_type NOT NULL,
    product_id UUID NULL,
    category_id UUID NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT chk_user_or_session CHECK (user_id IS NOT NULL OR session_id IS NOT NULL)
);

-- Indexes for fast querying by user, session, and recency
CREATE INDEX IF NOT EXISTS idx_customer_activities_user ON customer_activities(user_id);
CREATE INDEX IF NOT EXISTS idx_customer_activities_session ON customer_activities(session_id);
CREATE INDEX IF NOT EXISTS idx_customer_activities_product ON customer_activities(product_id);
CREATE INDEX IF NOT EXISTS idx_customer_activities_created ON customer_activities(created_at DESC);

-- 3. Idempotency table renamed to outbox_events
CREATE TABLE IF NOT EXISTS outbox_events (
    event_id VARCHAR(128) PRIMARY KEY,
    event_type VARCHAR(64) NOT NULL,
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
