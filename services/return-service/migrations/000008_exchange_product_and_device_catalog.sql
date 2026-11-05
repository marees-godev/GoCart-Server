-- 000008_exchange_product_and_device_catalog.sql
-- Add product_id and exchange_device_catalog_id, and drop order_id / order_item_id from exchanges

DROP INDEX IF EXISTS idx_exchanges_order_id;

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS order_item_id,
    ADD COLUMN IF NOT EXISTS product_id UUID,
    ADD COLUMN IF NOT EXISTS exchange_device_catalog_id UUID REFERENCES exchange_device_catalog(id);

CREATE INDEX IF NOT EXISTS idx_exchanges_product_id ON exchanges(product_id);
CREATE INDEX IF NOT EXISTS idx_exchanges_device_catalog_id ON exchanges(exchange_device_catalog_id);
