-- Create partner_approval_status ENUM
DO $$ BEGIN
    CREATE TYPE partner_approval_status AS ENUM (
        'PENDING',
        'APPROVED',
        'REJECTED',
        'APPEALED'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Add approval, appeal, and retention columns to delivery_partner_profiles
ALTER TABLE delivery_partner_profiles
    ADD COLUMN IF NOT EXISTS approval_status partner_approval_status NOT NULL DEFAULT 'PENDING',
    ADD COLUMN IF NOT EXISTS rejection_reason TEXT,
    ADD COLUMN IF NOT EXISTS appeal_reason TEXT,
    ADD COLUMN IF NOT EXISTS appealed_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

-- Add shipping_address_id column to deliveries table
ALTER TABLE deliveries
    ADD COLUMN IF NOT EXISTS shipping_address_id UUID;

-- Create indexes for performance and queries
CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_approval_deleted ON delivery_partner_profiles(approval_status, deleted_at);
CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_is_active ON delivery_partner_profiles(is_active);
CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_deactivated_at ON delivery_partner_profiles(deactivated_at);
CREATE INDEX IF NOT EXISTS idx_deliveries_shipping_address_id ON deliveries(shipping_address_id);
