ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS variant_id UUID;
ALTER TABLE cart_items ADD COLUMN IF NOT EXISTS store_id UUID;

DROP INDEX IF EXISTS idx_cart_items_unique_variant;
CREATE UNIQUE INDEX IF NOT EXISTS idx_cart_items_unique_variant ON cart_items(cart_id, product_id, COALESCE(variant_id, '00000000-0000-0000-0000-000000000000'::uuid));
