DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_approval_status') THEN
        CREATE TYPE store_approval_status AS ENUM ('PENDING', 'APPROVED', 'REJECTED', 'SUSPENDED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_kyc_status') THEN
        CREATE TYPE store_kyc_status AS ENUM ('PENDING', 'SUBMITTED', 'VERIFIED', 'REJECTED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'outbox_status') THEN
        CREATE TYPE outbox_status AS ENUM ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'bank_account_type') THEN
        CREATE TYPE bank_account_type AS ENUM ('SAVINGS', 'CURRENT', 'BUSINESS');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS stores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    description TEXT,
    logo_url TEXT,
    banner_url TEXT,
    address JSONB,
    approval_status store_approval_status NOT NULL DEFAULT 'PENDING',
    publish_status BOOLEAN NOT NULL DEFAULT FALSE,
    rejection_reason TEXT,
    kyc_status store_kyc_status NOT NULL DEFAULT 'PENDING',
    avg_store_rating NUMERIC(3,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS store_bank_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    account_holder_name VARCHAR(255) NOT NULL,
    account_number VARCHAR(100) NOT NULL,
    routing_number VARCHAR(100),
    bank_name VARCHAR(255) NOT NULL,
    account_type bank_account_type NOT NULL DEFAULT 'CURRENT',
    branch_code VARCHAR(100),
    is_primary BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS outbox_events (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    aggregate_type VARCHAR(100) NOT NULL,
    aggregate_id VARCHAR(255) NOT NULL,
    event_type VARCHAR(100) NOT NULL,
    payload JSONB NOT NULL,
    status outbox_status NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    published_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_stores_merchant_id ON stores(merchant_id);
CREATE INDEX IF NOT EXISTS idx_stores_slug ON stores(slug);
CREATE INDEX IF NOT EXISTS idx_stores_approval_status ON stores(approval_status);
CREATE INDEX IF NOT EXISTS idx_stores_publish_status ON stores(publish_status);
CREATE INDEX IF NOT EXISTS idx_store_bank_accounts_store_id ON store_bank_accounts(store_id);
CREATE INDEX IF NOT EXISTS idx_store_outbox_status_created ON outbox_events(status, created_at);

