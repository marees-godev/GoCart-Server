-- Add deletion_reason column to delivery_partner_profiles table
ALTER TABLE delivery_partner_profiles
    ADD COLUMN IF NOT EXISTS deletion_reason TEXT;

