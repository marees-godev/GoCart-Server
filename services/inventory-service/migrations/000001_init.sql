DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'inventory_transaction_type') THEN
        CREATE TYPE inventory_transaction_type AS ENUM ('RESTOCK', 'RESERVE', 'RELEASE', 'ADJUSTMENT', 'SALE', 'RETURN');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'reservation_status') THEN
        CREATE TYPE reservation_status AS ENUM ('RESERVED', 'CONFIRMED', 'RELEASED', 'EXPIRED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'outbox_status') THEN
        CREATE TYPE outbox_status AS ENUM ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS inventories (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL,
    variant_id UUID,
    sku VARCHAR(100) NOT NULL UNIQUE,
    available_quantity INT NOT NULL DEFAULT 0 CHECK (available_quantity >= 0),
    reserved_quantity INT NOT NULL DEFAULT 0 CHECK (reserved_quantity >= 0),
    low_stock_threshold INT NOT NULL DEFAULT 5 CHECK (low_stock_threshold >= 0),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory_reservations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL,
    product_id UUID NOT NULL,
    variant_id UUID,
    quantity INT NOT NULL CHECK (quantity > 0),
    status reservation_status NOT NULL DEFAULT 'RESERVED',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS inventory_transactions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    inventory_id UUID NOT NULL REFERENCES inventories(id) ON DELETE CASCADE,
    type inventory_transaction_type NOT NULL,
    quantity INT NOT NULL,
    reference_id VARCHAR(255),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status outbox_status NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_inventories_product_null_variant ON inventories(product_id) WHERE variant_id IS NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_inventories_product_variant ON inventories(product_id, variant_id) WHERE variant_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS idx_inventories_sku ON inventories(sku);
CREATE INDEX IF NOT EXISTS idx_inv_res_order_id ON inventory_reservations(order_id);
CREATE INDEX IF NOT EXISTS idx_inv_res_product_id ON inventory_reservations(product_id);
CREATE INDEX IF NOT EXISTS idx_inv_res_variant_id ON inventory_reservations(variant_id);
CREATE INDEX IF NOT EXISTS idx_inv_res_status ON inventory_reservations(status);
CREATE INDEX IF NOT EXISTS idx_inv_tx_inventory_id ON inventory_transactions(inventory_id);
CREATE INDEX IF NOT EXISTS idx_inv_tx_created_at ON inventory_transactions(created_at);
CREATE INDEX IF NOT EXISTS idx_inventory_outbox_status_created ON outbox_events(status, created_at);

