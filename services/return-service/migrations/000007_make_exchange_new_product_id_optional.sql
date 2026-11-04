-- 000007_make_exchange_new_product_id_optional.sql
-- Allow initiating exchanges without specifying a new product ID up-front

ALTER TABLE exchanges
    ALTER COLUMN new_product_id DROP NOT NULL;

