DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_exchanges_quote_id'
    ) THEN
        ALTER TABLE exchanges ADD CONSTRAINT uq_exchanges_quote_id UNIQUE (quote_id);
    END IF;
END $$;

