-- 000005_product_reports.sql
-- Product reporting and moderation

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'report_reason') THEN
        CREATE TYPE report_reason AS ENUM (
            'INAPPROPRIATE_CONTENT',
            'MISLEADING_INFORMATION',
            'COUNTERFEIT_OR_FRAUDULENT',
            'PROHIBITED_PRODUCT',
            'SPAM_OR_DUPLICATE',
            'OTHER'
        );
    END IF;
END $$;

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'report_status') THEN
        CREATE TYPE report_status AS ENUM (
            'PENDING',
            'REVIEWED',
            'DISMISSED',
            'ACTION_TAKEN'
        );
    END IF;
END $$;

CREATE TABLE IF NOT EXISTS product_reports (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    product_id UUID NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    reporter_id UUID NOT NULL,
    reason report_reason NOT NULL,
    description TEXT,
    status report_status NOT NULL DEFAULT 'PENDING',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_product_reports_user_product UNIQUE (product_id, reporter_id)
);

CREATE INDEX IF NOT EXISTS idx_product_reports_product_id ON product_reports(product_id);
CREATE INDEX IF NOT EXISTS idx_product_reports_reporter_id ON product_reports(reporter_id);
CREATE INDEX IF NOT EXISTS idx_product_reports_status ON product_reports(status);
CREATE INDEX IF NOT EXISTS idx_product_reports_created_at ON product_reports(created_at DESC);
