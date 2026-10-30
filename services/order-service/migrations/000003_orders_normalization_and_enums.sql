DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_status') THEN
        CREATE TYPE order_status AS ENUM (
            'CREATED',
            'PENDING_PAYMENT',
            'PAID',
            'CONFIRMED',
            'PROCESSING',
            'SHIPPED',
            'OUT_FOR_DELIVERY',
            'DELIVERED',
            'PAYMENT_FAILED',
            'CANCELLED',
            'RETURNED',
            'RETURN_REQUESTED',
            'REFUNDED'
        );
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'order_payment_status') THEN
        CREATE TYPE order_payment_status AS ENUM (
            'PENDING',
            'PAID',
            'FAILED',
            'REFUNDED'
        );
    END IF;
END $$;

ALTER TABLE orders
    ADD COLUMN IF NOT EXISTS parent_order_id UUID,
    ADD COLUMN IF NOT EXISTS store_id UUID,
    ADD COLUMN IF NOT EXISTS shipping_fee DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (shipping_fee >= 0),
    ADD COLUMN IF NOT EXISTS shipping_address_id UUID,
    ADD COLUMN IF NOT EXISTS billing_address_id UUID;

ALTER TABLE orders
    ALTER COLUMN shipping_address DROP NOT NULL,
    ALTER COLUMN shipping_address SET DEFAULT '{}'::jsonb,
    ALTER COLUMN billing_address DROP NOT NULL,
    ALTER COLUMN billing_address SET DEFAULT '{}'::jsonb;

-- Safely convert status column to order_status ENUM if it is varchar
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'orders' AND column_name = 'status' AND udt_name = 'varchar'
    ) THEN
        ALTER TABLE orders ALTER COLUMN status DROP DEFAULT;
        ALTER TABLE orders ALTER COLUMN status TYPE order_status USING (status::order_status);
        ALTER TABLE orders ALTER COLUMN status SET DEFAULT 'CREATED'::order_status;
    END IF;
END $$;

-- Safely convert payment_status column to order_payment_status ENUM if it is varchar
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'orders' AND column_name = 'payment_status' AND udt_name = 'varchar'
    ) THEN
        ALTER TABLE orders ALTER COLUMN payment_status DROP DEFAULT;
        ALTER TABLE orders ALTER COLUMN payment_status TYPE order_payment_status USING (payment_status::order_payment_status);
        ALTER TABLE orders ALTER COLUMN payment_status SET DEFAULT 'PENDING'::order_payment_status;
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_orders_parent_order_id ON orders(parent_order_id);
CREATE INDEX IF NOT EXISTS idx_orders_store_id ON orders(store_id);
CREATE INDEX IF NOT EXISTS idx_orders_shipping_address_id ON orders(shipping_address_id);
CREATE INDEX IF NOT EXISTS idx_orders_billing_address_id ON orders(billing_address_id);
