-- 000004_add_product_weight.sql
-- Add product weight column in order to calculate shipping fee.

ALTER TABLE products
    ADD COLUMN IF NOT EXISTS weight DECIMAL(10,3) NOT NULL DEFAULT 0.00 CHECK (weight >= 0);
