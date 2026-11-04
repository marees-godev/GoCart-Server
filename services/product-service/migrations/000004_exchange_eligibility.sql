-- 000004_exchange_eligibility.sql
-- Add exchange eligibility configuration columns to products table

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS is_exchange_eligible BOOLEAN NOT NULL DEFAULT FALSE,
    ADD COLUMN IF NOT EXISTS max_exchange_discount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (max_exchange_discount >= 0);

CREATE INDEX IF NOT EXISTS idx_products_exchange_eligible ON products(is_exchange_eligible) WHERE is_exchange_eligible = TRUE;

