-- Migration 000005: Add PROCESSED to refund_status enum
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_enum 
        WHERE enumtypid = 'refund_status'::regtype 
        AND enumlabel = 'PROCESSED'
    ) THEN
        ALTER TYPE refund_status ADD VALUE 'PROCESSED';
    END IF;
END $$;
