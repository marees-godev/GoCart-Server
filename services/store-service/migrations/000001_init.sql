DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'store_approval_status') THEN
        CREATE TYPE store_approval_status AS ENUM ('DRAFT', 'PENDING_APPROVAL', 'APPROVED', 'REJECTED', 'SUSPENDED', 'CLOSED');
    END IF;
END $$;

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'outbox_status') THEN
        CREATE TYPE outbox_status AS ENUM ('PENDING', 'PROCESSING', 'PUBLISHED', 'FAILED');
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS stores (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    merchant_id UUID NOT NULL,
    name VARCHAR(255) NOT NULL,
    slug VARCHAR(255) NOT NULL UNIQUE,
    business_email VARCHAR(255),
    business_phone VARCHAR(50),
    description TEXT,
    logo_url TEXT,
    address TEXT,
    is_vacation_mode BOOLEAN NOT NULL DEFAULT FALSE,
    approval_status store_approval_status NOT NULL DEFAULT 'DRAFT',
    rejection_reason TEXT,
    avg_store_rating NUMERIC(3,2) NOT NULL DEFAULT 0.00,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS store_bank_accounts (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    store_id UUID NOT NULL REFERENCES stores(id) ON DELETE CASCADE,
    account_holder_name VARCHAR(255) NOT NULL,
    bank_name VARCHAR(255) NOT NULL,
    account_number VARCHAR(100) NOT NULL,
    routing_number VARCHAR(100),
    tax_id VARCHAR(100),
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
CREATE INDEX IF NOT EXISTS idx_stores_is_vacation_mode ON stores(is_vacation_mode);
CREATE INDEX IF NOT EXISTS idx_store_bank_accounts_store_id ON store_bank_accounts(store_id);
CREATE INDEX IF NOT EXISTS idx_store_outbox_status_created ON outbox_events(status, created_at);

