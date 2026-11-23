-- 000008_add_remember_me_to_refresh_tokens.sql
-- Add remember_me column to refresh_tokens table to support configurable session lifetimes

ALTER TABLE refresh_tokens ADD COLUMN IF NOT EXISTS remember_me BOOLEAN NOT NULL DEFAULT FALSE;
