-- 000010_drop_exchange_order_columns.sql
-- Drop order_id and order_item_id columns from exchanges table

DROP INDEX IF EXISTS idx_exchanges_order_id;

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS order_item_id;

