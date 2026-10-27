-- 1. Ensure all order_status enum values exist
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
            'RETURN_REQUESTED',
            'RETURNED',
            'REFUNDED'
        );
    END IF;
END $$;

ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'PENDING_PAYMENT';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'PAID';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'OUT_FOR_DELIVERY';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'PAYMENT_FAILED';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'RETURN_REQUESTED';
ALTER TYPE order_status ADD VALUE IF NOT EXISTS 'REFUNDED';

-- 2. Traceable order status history table
CREATE TABLE IF NOT EXISTS order_status_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    order_id UUID NOT NULL REFERENCES orders(id) ON DELETE CASCADE,
    from_status order_status,
    to_status order_status NOT NULL,
    reason TEXT,
    changed_by VARCHAR(255),
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_order_status_history_order_id ON order_status_history(order_id);
CREATE INDEX IF NOT EXISTS idx_order_status_history_created_at ON order_status_history(created_at);
CREATE INDEX IF NOT EXISTS idx_orders_idempotency_key ON orders(idempotency_key);
