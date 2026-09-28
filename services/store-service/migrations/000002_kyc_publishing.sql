DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_kyc_status') THEN
        CREATE TYPE store_kyc_status AS ENUM ('NOT_SUBMITTED', 'PENDING', 'VERIFIED', 'REJECTED');
    END IF;
END $$;

ALTER TABLE stores ADD COLUMN IF NOT EXISTS is_published BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE stores ADD COLUMN IF NOT EXISTS kyc_status store_kyc_status NOT NULL DEFAULT 'NOT_SUBMITTED';
ALTER TABLE stores ADD COLUMN IF NOT EXISTS gstin VARCHAR(15);

DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'store_bank_accounts' AND column_name = 'routing_number'
    ) THEN
        ALTER TABLE store_bank_accounts RENAME COLUMN routing_number TO ifsc_code;
    END IF;
END $$;

ALTER TABLE store_bank_accounts ADD COLUMN IF NOT EXISTS ifsc_code VARCHAR(100);
ALTER TABLE store_bank_accounts DROP COLUMN IF EXISTS tax_id;
ALTER TABLE store_bank_accounts DROP COLUMN IF EXISTS business_registration;
ALTER TABLE stores DROP COLUMN IF EXISTS business_registration;
