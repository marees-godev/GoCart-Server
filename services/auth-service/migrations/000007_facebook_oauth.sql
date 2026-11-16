-- 000007_facebook_oauth.sql
-- Add FACEBOOK provider to auth_provider_type enum

ALTER TYPE auth_provider_type ADD VALUE IF NOT EXISTS 'FACEBOOK';
