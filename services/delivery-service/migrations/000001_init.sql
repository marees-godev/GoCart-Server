DO $$ BEGIN
    CREATE TYPE delivery_status AS ENUM (
        'CREATED',
        'PICKED_UP',
        'IN_TRANSIT',
        'OUT_FOR_DELIVERY',
        'DELIVERED',
        'FAILED',
        'CANCELLED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE outbox_status AS ENUM (
        'PENDING',
        'PROCESSING',
        'PUBLISHED',
        'FAILED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

CREATE TABLE IF NOT EXISTS deliveries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL UNIQUE,
    delivery_partner VARCHAR(100) NOT NULL DEFAULT 'STANDARD',
    tracking_number VARCHAR(100) NOT NULL UNIQUE,
    status delivery_status NOT NULL DEFAULT 'CREATED',
    street VARCHAR(255) NOT NULL DEFAULT '',
    city VARCHAR(100) NOT NULL DEFAULT '',
    state VARCHAR(100) NOT NULL DEFAULT '',
    postal_code VARCHAR(50) NOT NULL DEFAULT '',
    country VARCHAR(100) NOT NULL DEFAULT '',
    contact_number VARCHAR(50) NOT NULL DEFAULT '',
    shipping_address JSONB NOT NULL DEFAULT '{}'::jsonb,
    estimated_delivery TIMESTAMPTZ,
    actual_delivery TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS delivery_tracking_updates (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    delivery_id UUID NOT NULL REFERENCES deliveries(id) ON DELETE CASCADE,
    status delivery_status NOT NULL,
    location VARCHAR(255),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL DEFAULT 'delivery',
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    headers JSONB NOT NULL DEFAULT '{}'::jsonb,
    topic VARCHAR(255) NOT NULL DEFAULT '',
    status outbox_status NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    next_retry_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_deliveries_order_id ON deliveries(order_id);
CREATE UNIQUE INDEX IF NOT EXISTS idx_deliveries_tracking_number ON deliveries(tracking_number);
CREATE INDEX IF NOT EXISTS idx_deliveries_status ON deliveries(status);
CREATE INDEX IF NOT EXISTS idx_delivery_tracking_delivery_id ON delivery_tracking_updates(delivery_id);
CREATE INDEX IF NOT EXISTS idx_delivery_outbox_status_created ON outbox_events(status, created_at ASC);
CREATE INDEX IF NOT EXISTS idx_delivery_outbox_pending_created ON outbox_events(created_at ASC) WHERE status = 'PENDING';
CREATE INDEX IF NOT EXISTS idx_delivery_outbox_aggregate ON outbox_events(aggregate_id);
