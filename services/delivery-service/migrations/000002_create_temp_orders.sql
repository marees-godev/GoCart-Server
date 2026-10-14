-- 1. Ensure deliveries table has all columns if it was created under an earlier schema
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS delivery_partner VARCHAR(100) NOT NULL DEFAULT 'STANDARD';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS street VARCHAR(255) NOT NULL DEFAULT '';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS city VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS state VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS postal_code VARCHAR(50) NOT NULL DEFAULT '';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS country VARCHAR(100) NOT NULL DEFAULT '';
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS contact_number VARCHAR(50) NOT NULL DEFAULT '';

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
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'deliveries' AND column_name = 'status' AND data_type = 'character varying'
    ) THEN
        ALTER TABLE deliveries ALTER COLUMN status DROP DEFAULT;
        ALTER TABLE deliveries ALTER COLUMN status TYPE delivery_status USING status::delivery_status;
        ALTER TABLE deliveries ALTER COLUMN status SET DEFAULT 'CREATED'::delivery_status;
    END IF;
END $$;

-- 2. Create ENUM for temp_order_status
DO $$ BEGIN
    CREATE TYPE temp_order_status AS ENUM (
        'PENDING',
        'CONFIRMED',
        'PROCESSING',
        'SHIPPED',
        'DELIVERED',
        'CANCELLED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- 3. Create temp_orders table
CREATE TABLE IF NOT EXISTS temp_orders (
    id UUID PRIMARY KEY,
    order_number VARCHAR(50) NOT NULL UNIQUE,
    customer_id UUID,
    status temp_order_status NOT NULL DEFAULT 'CONFIRMED',
    total_amount NUMERIC(12, 2) NOT NULL DEFAULT 0.00 CHECK (total_amount >= 0),
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    street VARCHAR(255) NOT NULL DEFAULT '',
    city VARCHAR(100) NOT NULL DEFAULT '',
    state VARCHAR(100) NOT NULL DEFAULT '',
    postal_code VARCHAR(50) NOT NULL DEFAULT '',
    country VARCHAR(100) NOT NULL DEFAULT '',
    contact_number VARCHAR(50) NOT NULL DEFAULT '',
    shipping_address JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_temp_orders_order_number ON temp_orders(order_number);
CREATE INDEX IF NOT EXISTS idx_temp_orders_status ON temp_orders(status);
CREATE INDEX IF NOT EXISTS idx_temp_orders_customer_id ON temp_orders(customer_id);

-- 4. Backfill existing deliveries into temp_orders and apply foreign key
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'fk_deliveries_temp_orders'
    ) THEN
        INSERT INTO temp_orders (
            id, order_number, street, city, state, postal_code, country, contact_number, shipping_address, created_at, updated_at
        )
        SELECT 
            order_id,
            'ORD-' || UPPER(REPLACE(order_id::text, '-', '')),
            street,
            city,
            state,
            postal_code,
            country,
            contact_number,
            shipping_address,
            created_at,
            updated_at
        FROM deliveries
        ON CONFLICT (id) DO NOTHING;

        ALTER TABLE deliveries
            ADD CONSTRAINT fk_deliveries_temp_orders
            FOREIGN KEY (order_id)
            REFERENCES temp_orders(id)
            ON DELETE CASCADE;
    END IF;
END $$;
