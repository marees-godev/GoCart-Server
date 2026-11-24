-- Ensure confirmed orders require payment_status to be PAID or REFUNDED
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'chk_orders_confirmed_requires_paid'
    ) THEN
        ALTER TABLE orders
            ADD CONSTRAINT chk_orders_confirmed_requires_paid
            CHECK (status != 'CONFIRMED' OR payment_status IN ('PAID', 'REFUNDED'));
    END IF;
END $$;
