-- Migration 000004: Add REFUND_PENDING and PARTIALLY_REFUNDED to payment_status enum
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum 
        WHERE enumtypid = 'payment_status'::regtype 
        AND enumlabel = 'REFUND_PENDING'
    ) THEN
        ALTER TYPE payment_status ADD VALUE 'REFUND_PENDING';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM pg_enum 
        WHERE enumtypid = 'payment_status'::regtype 
        AND enumlabel = 'PARTIALLY_REFUNDED'
    ) THEN
        ALTER TYPE payment_status ADD VALUE 'PARTIALLY_REFUNDED';
    END IF;
END $$;
