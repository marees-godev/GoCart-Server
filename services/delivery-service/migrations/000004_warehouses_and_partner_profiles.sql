-- 1. Create warehouses table
CREATE TABLE IF NOT EXISTS warehouses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name VARCHAR(255) NOT NULL,
    code VARCHAR(50) NOT NULL UNIQUE,
    street VARCHAR(255) NOT NULL DEFAULT '',
    city VARCHAR(100) NOT NULL DEFAULT '',
    state VARCHAR(100) NOT NULL DEFAULT '',
    postal_code VARCHAR(50) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_warehouses_is_active ON warehouses(is_active);
CREATE INDEX IF NOT EXISTS idx_warehouses_city ON warehouses(city);
CREATE INDEX IF NOT EXISTS idx_warehouses_state ON warehouses(state);

-- 2. Create delivery_partner_profiles table
CREATE TABLE IF NOT EXISTS delivery_partner_profiles (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL UNIQUE,
    company_name VARCHAR(255) NOT NULL DEFAULT '',
    email VARCHAR(255) NOT NULL DEFAULT '',
    contact_number VARCHAR(50) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_partner_id ON delivery_partner_profiles(partner_id);
CREATE INDEX IF NOT EXISTS idx_delivery_partner_profiles_is_active ON delivery_partner_profiles(is_active);

-- 3. Create partner_warehouses association table
CREATE TABLE IF NOT EXISTS partner_warehouses (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    partner_id UUID NOT NULL REFERENCES delivery_partner_profiles(partner_id) ON DELETE CASCADE,
    warehouse_id UUID NOT NULL REFERENCES warehouses(id) ON DELETE CASCADE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(partner_id, warehouse_id)
);

CREATE INDEX IF NOT EXISTS idx_partner_warehouses_partner_id ON partner_warehouses(partner_id);
CREATE INDEX IF NOT EXISTS idx_partner_warehouses_warehouse_id ON partner_warehouses(warehouse_id);

-- 4. Ensure estimated_delivery column exists in deliveries
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS estimated_delivery TIMESTAMPTZ;
ALTER TABLE deliveries ADD COLUMN IF NOT EXISTS actual_delivery TIMESTAMPTZ;
