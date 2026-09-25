DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_kyc_status') THEN
        CREATE TYPE store_kyc_status AS ENUM ('NOT_SUBMITTED', 'PENDING', 'VERIFIED', 'REJECTED');
    END IF;
END $$;

ALTER TABLE stores ADD COLUMN IF NOT EXISTS is_published BOOLEAN NOT NULL DEFAULT FALSE;
ALTER TABLE stores ADD COLUMN IF NOT EXISTS kyc_status store_kyc_status NOT NULL DEFAULT 'NOT_SUBMITTED';
ALTER TABLE stores ADD COLUMN IF NOT EXISTS gstin VARCHAR(15);


ALTER TABLE store_bank_accounts RENAME COLUMN routing_number TO ifsc_code;
ALTER TABLE store_bank_accounts DROP COLUMN IF EXISTS tax_id;
ALTER TABLE store_bank_accounts DROP COLUMN IF EXISTS business_registration;
ALTER TABLE stores DROP COLUMN IF EXISTS business_registration;
