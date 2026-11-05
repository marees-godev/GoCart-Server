-- 000007_make_exchange_new_product_id_optional.sql
-- Drop new_product_id column from exchanges

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS new_product_id;


-- Exchange_product_and_device_catalog.sql
-- Add product_id and exchange_device_catalog_id, and drop order_id / order_item_id from exchanges

DROP INDEX IF EXISTS idx_exchanges_order_id;

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS order_item_id,
    ADD COLUMN IF NOT EXISTS product_id UUID,
    ADD COLUMN IF NOT EXISTS exchange_device_catalog_id UUID REFERENCES exchange_device_catalog(id);

CREATE INDEX IF NOT EXISTS idx_exchanges_product_id ON exchanges(product_id);
CREATE INDEX IF NOT EXISTS idx_exchanges_device_catalog_id ON exchanges(exchange_device_catalog_id);


DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_exchanges_quote_id'
    ) THEN
        ALTER TABLE exchanges ADD CONSTRAINT uq_exchanges_quote_id UNIQUE (quote_id);
    END IF;
END $$;


-- Drop_exchange_order_columns.sql
-- Drop order_id and order_item_id columns from exchanges table

DROP INDEX IF EXISTS idx_exchanges_order_id;

ALTER TABLE exchanges
    DROP COLUMN IF EXISTS order_id,
    DROP COLUMN IF EXISTS order_item_id;



-- Drop_new_product_id.sql
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











