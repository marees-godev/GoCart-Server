-- 000006_drop_temp_orders.sql
-- Remove temp_orders table and associated constraint from delivery service

-- 1. Drop foreign key constraint on deliveries referencing temp_orders
ALTER TABLE deliveries DROP CONSTRAINT IF EXISTS fk_deliveries_temp_orders;

-- 2. Drop indexes associated with temp_orders
DROP INDEX IF EXISTS idx_temp_orders_order_number;
DROP INDEX IF EXISTS idx_temp_orders_status;
DROP INDEX IF EXISTS idx_temp_orders_customer_id;

-- 3. Drop temp_orders table
DROP TABLE IF EXISTS temp_orders CASCADE;

-- 4. Drop temp_order_status ENUM type
DROP TYPE IF EXISTS temp_order_status CASCADE;
