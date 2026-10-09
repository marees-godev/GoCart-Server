-- Ensure PostgreSQL ENUM types exist if missing
DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'payment_status') THEN
        CREATE TYPE payment_status AS ENUM ('INITIATED', 'PENDING', 'SUCCESS', 'FAILED', 'CANCELLED', 'REFUNDED');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'refund_status') THEN
        CREATE TYPE refund_status AS ENUM ('PENDING', 'SUCCESS', 'FAILED');
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'outbox_status') THEN
        CREATE TYPE outbox_status AS ENUM ('PENDING', 'PUBLISHED', 'FAILED');
    END IF;
END $$;

-- Add missing payment_method and payment_method_id columns to payments table
ALTER TABLE payments
    ADD COLUMN IF NOT EXISTS payment_method VARCHAR(50) NOT NULL DEFAULT 'CREDIT_CARD',
    ADD COLUMN IF NOT EXISTS payment_method_id UUID REFERENCES payment_methods(id) ON DELETE RESTRICT;

INSERT INTO payment_methods (code, name) VALUES
('NET_BANKING', 'Net Banking')
ON CONFLICT (code) DO NOTHING;

-- Index for searching payments by gateway transaction ID (Razorpay Order ID)
CREATE INDEX IF NOT EXISTS idx_payments_gateway_transaction_id ON payments(gateway_transaction_id);

