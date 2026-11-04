-- Add gstin, is_gstin_verified, and deactivation_reason columns to delivery_partner_profiles table
ALTER TABLE delivery_partner_profiles
    ADD COLUMN IF NOT EXISTS gstin VARCHAR(15),
    ADD COLUMN IF NOT EXISTS is_gstin_verified BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS deactivation_reason TEXT;

CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_gstin ON delivery_partner_profiles(gstin);
