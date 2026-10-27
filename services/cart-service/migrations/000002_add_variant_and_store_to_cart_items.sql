ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS variant_id UUID;
ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS store_id UUID;

-- Drop legacy table constraint if it exists from older schema
ALTER TABLE cart_items DROP CONSTRAINT IF EXISTS cart_items_cart_id_product_id_key;

-- Convert any existing 000000 sentinel values to NULL
UPDATE cart_items SET variant_id = NULL WHERE variant_id = '00000000-0000-0000-0000-000000000000'::uuid;

-- Recreate unique index with NULLS NOT DISTINCT (PostgreSQL 15+)
DROP INDEX IF EXISTS idx_cart_items_unique_variant;
CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_unique_variant ON cart_items(cart_id, product_id, variant_id) NULLS NOT DISTINCT;

