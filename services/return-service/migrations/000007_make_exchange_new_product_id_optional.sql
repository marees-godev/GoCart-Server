-- 000007_make_exchange_new_product_id_optional.sql
-- Drop new_product_id column from exchanges

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS new_product_id;
