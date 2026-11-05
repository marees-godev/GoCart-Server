-- 000011_drop_new_product_id.sql
-- Remove new_product_id column from exchanges and exchange_quotes

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS new_product_id;

ALTER TABLE exchange_quotes
    ADD COLUMN IF NOT EXISTS product_id UUID;

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'exchange_quotes' AND column_name = 'new_product_id'
    ) THEN
        UPDATE exchange_quotes SET product_id = new_product_id WHERE product_id IS NULL;
        ALTER TABLE exchange_quotes DROP COLUMN new_product_id;
    END IF;
END $$;

