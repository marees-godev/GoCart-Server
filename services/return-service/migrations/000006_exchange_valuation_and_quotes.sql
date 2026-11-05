-- 000006_exchange_valuation_and_quotes.sql
-- Relational diagnostic questionnaires, device catalog, base valuation matrix, and locked exchange quotes

DO $$ BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'exchange_quote_status') THEN
        CREATE TYPE exchange_quote_status AS ENUM (
            'QUOTED',
            'LOCKED',
            'APPLIED',
            'EXPIRED',
            'CANCELLED'
        );
    END IF;
END $$;

ALTER TYPE exchange_status ADD VALUE IF NOT EXISTS 'EXCHANGE_REQUESTED';

CREATE TABLE IF NOT EXISTS exchange_device_catalog (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_code VARCHAR(50) NOT NULL,
    brand VARCHAR(100) NOT NULL,
    model VARCHAR(100) NOT NULL,
    variant VARCHAR(100) NOT NULL DEFAULT '',
    base_exchange_value DECIMAL(12,2) NOT NULL CHECK (base_exchange_value >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_device_catalog UNIQUE (category_code, brand, model, variant)
);

CREATE TABLE IF NOT EXISTS exchange_diagnostic_questions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    category_code VARCHAR(50) NOT NULL,
    question_key VARCHAR(100) NOT NULL,
    question_text VARCHAR(255) NOT NULL,
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_category_question UNIQUE (category_code, question_key)
);

CREATE TABLE IF NOT EXISTS exchange_question_options (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    question_id UUID NOT NULL REFERENCES exchange_diagnostic_questions(id) ON DELETE CASCADE,
    option_key VARCHAR(100) NOT NULL,
    option_label VARCHAR(255) NOT NULL,
    deduction_amount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (deduction_amount >= 0),
    is_fatal_disqualifier BOOLEAN NOT NULL DEFAULT FALSE,
    rejection_reason VARCHAR(255),
    display_order INT NOT NULL DEFAULT 0,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_question_option UNIQUE (question_id, option_key)
);

CREATE TABLE IF NOT EXISTS exchange_quotes (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    new_product_id UUID NOT NULL,
    category_code VARCHAR(50) NOT NULL,
    old_brand VARCHAR(100) NOT NULL,
    old_model VARCHAR(100) NOT NULL,
    old_variant VARCHAR(100),
    serial_or_imei VARCHAR(100),
    base_exchange_value DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (base_exchange_value >= 0),
    total_deductions DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (total_deductions >= 0),
    final_discount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (final_discount >= 0),
    is_disqualified BOOLEAN NOT NULL DEFAULT FALSE,
    disqualification_reason TEXT,
    status exchange_quote_status NOT NULL DEFAULT 'QUOTED',
    expires_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS exchange_quote_answers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    quote_id UUID NOT NULL REFERENCES exchange_quotes(id) ON DELETE CASCADE,
    question_key VARCHAR(100) NOT NULL,
    option_key VARCHAR(100) NOT NULL,
    deduction_amount DECIMAL(12,2) NOT NULL DEFAULT 0.00 CHECK (deduction_amount >= 0),
    is_fatal BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

ALTER TABLE exchanges
    ADD COLUMN IF NOT EXISTS quote_id UUID REFERENCES exchange_quotes(id);

CREATE INDEX IF NOT EXISTS idx_exchange_device_catalog_cat ON exchange_device_catalog(category_code, is_active);
CREATE INDEX IF NOT EXISTS idx_exchange_diagnostic_questions_cat ON exchange_diagnostic_questions(category_code, display_order);
CREATE INDEX IF NOT EXISTS idx_exchange_question_options_qid ON exchange_question_options(question_id, display_order);
CREATE INDEX IF NOT EXISTS idx_exchange_quotes_user ON exchange_quotes(user_id);
CREATE INDEX IF NOT EXISTS idx_exchange_quotes_status ON exchange_quotes(status);
CREATE INDEX IF NOT EXISTS idx_exchange_quotes_expires_at ON exchange_quotes(expires_at);
CREATE INDEX IF NOT EXISTS idx_exchange_quote_answers_quote ON exchange_quote_answers(quote_id);
CREATE INDEX IF NOT EXISTS idx_exchanges_quote_id ON exchanges(quote_id);

-- Seed Initial Device Catalog
INSERT INTO exchange_device_catalog (category_code, brand, model, variant, base_exchange_value)
VALUES
    ('SMARTPHONE', 'Apple', 'iPhone 13', '128GB', 26000.00),
    ('SMARTPHONE', 'Apple', 'iPhone 14', '128GB', 32000.00),
    ('SMARTPHONE', 'Apple', 'iPhone 15', '128GB', 40000.00),
    ('SMARTPHONE', 'Samsung', 'Galaxy S22', '128GB', 22000.00),
    ('SMARTPHONE', 'Samsung', 'Galaxy S23', '128GB', 30000.00),
    ('SMARTPHONE', 'Google', 'Pixel 7', '128GB', 20000.00),
    ('SMARTPHONE', 'Google', 'Pixel 8', '128GB', 28000.00),
    ('LAPTOP', 'Apple', 'MacBook Air M1', '8GB/256GB', 35000.00),
    ('LAPTOP', 'Apple', 'MacBook Air M2', '8GB/256GB', 48000.00),
    ('LAPTOP', 'Dell', 'XPS 13', '16GB/512GB', 38000.00)
ON CONFLICT (category_code, brand, model, variant) DO NOTHING;

-- Seed Diagnostic Questions & Disqualifiers for SMARTPHONE
DO $$
DECLARE
    q_power UUID;
    q_screen UUID;
    q_cracks UUID;
    q_body UUID;
BEGIN
    INSERT INTO exchange_diagnostic_questions (category_code, question_key, question_text, display_order)
    VALUES ('SMARTPHONE', 'powers_on', 'Does the phone switch on and boot to the home screen?', 1)
    ON CONFLICT (category_code, question_key) DO UPDATE SET question_text = EXCLUDED.question_text
    RETURNING id INTO q_power;

    IF q_power IS NOT NULL THEN
        INSERT INTO exchange_question_options (question_id, option_key, option_label, deduction_amount, is_fatal_disqualifier, rejection_reason, display_order)
        VALUES
            (q_power, 'yes', 'Yes, powers on normally', 0.00, FALSE, NULL, 1),
            (q_power, 'no', 'No, does not turn on or is stuck in bootloop', 0.00, TRUE, 'Device does not power on. Items must power on to be eligible for exchange.', 2)
        ON CONFLICT (question_id, option_key) DO NOTHING;
    END IF;

    INSERT INTO exchange_diagnostic_questions (category_code, question_key, question_text, display_order)
    VALUES ('SMARTPHONE', 'screen_functional', 'Is the touchscreen and display completely functional without touch dead-zones?', 2)
    ON CONFLICT (category_code, question_key) DO UPDATE SET question_text = EXCLUDED.question_text
    RETURNING id INTO q_screen;

    IF q_screen IS NOT NULL THEN
        INSERT INTO exchange_question_options (question_id, option_key, option_label, deduction_amount, is_fatal_disqualifier, rejection_reason, display_order)
        VALUES
            (q_screen, 'yes', 'Yes, touch works across entire screen', 0.00, FALSE, NULL, 1),
            (q_screen, 'partial', 'Touch issues or ghost touch present', 3500.00, FALSE, NULL, 2),
            (q_screen, 'no', 'Touch not working at all', 0.00, TRUE, 'Touchscreen is completely unresponsive. Device is disqualified.', 3)
        ON CONFLICT (question_id, option_key) DO NOTHING;
    END IF;

    INSERT INTO exchange_diagnostic_questions (category_code, question_key, question_text, display_order)
    VALUES ('SMARTPHONE', 'screen_cracks', 'Are there any cracks, lines, or bleeding pixels on the screen?', 3)
    ON CONFLICT (category_code, question_key) DO UPDATE SET question_text = EXCLUDED.question_text
    RETURNING id INTO q_cracks;

    IF q_cracks IS NOT NULL THEN
        INSERT INTO exchange_question_options (question_id, option_key, option_label, deduction_amount, is_fatal_disqualifier, rejection_reason, display_order)
        VALUES
            (q_cracks, 'none', 'Flawless, no cracks or deep scratches', 0.00, FALSE, NULL, 1),
            (q_cracks, 'minor_scratches', 'Minor hairline scratches only', 1000.00, FALSE, NULL, 2),
            (q_cracks, 'cracked_glass', 'Cracked glass but display visible', 4500.00, FALSE, NULL, 3),
            (q_cracks, 'broken_lcd', 'Broken screen / display bleeding', 8000.00, FALSE, NULL, 4)
        ON CONFLICT (question_id, option_key) DO NOTHING;
    END IF;

    INSERT INTO exchange_diagnostic_questions (category_code, question_key, question_text, display_order)
    VALUES ('SMARTPHONE', 'body_condition', 'What is the physical condition of the back panel and body frame?', 4)
    ON CONFLICT (category_code, question_key) DO UPDATE SET question_text = EXCLUDED.question_text
    RETURNING id INTO q_body;

    IF q_body IS NOT NULL THEN
        INSERT INTO exchange_question_options (question_id, option_key, option_label, deduction_amount, is_fatal_disqualifier, rejection_reason, display_order)
        VALUES
            (q_body, 'flawless', 'No dents or scratches', 0.00, FALSE, NULL, 1),
            (q_body, 'minor_wear', 'Normal wear and tear / minor scuffs', 800.00, FALSE, NULL, 2),
            (q_body, 'dents', 'Visible dents or heavy scratches', 2000.00, FALSE, NULL, 3),
            (q_body, 'cracked_bent', 'Cracked back glass or bent chassis', 4000.00, FALSE, NULL, 4)
        ON CONFLICT (question_id, option_key) DO NOTHING;
    END IF;
END $$;

