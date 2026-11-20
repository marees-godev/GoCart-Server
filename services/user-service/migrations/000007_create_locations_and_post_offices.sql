CREATE TABLE IF NOT EXISTS locations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    pincode VARCHAR(10) NOT NULL UNIQUE,
    country VARCHAR(100) NOT NULL,
    state VARCHAR(100) NOT NULL,
    district VARCHAR(100) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_locations_pincode ON locations(pincode);

CREATE TABLE IF NOT EXISTS location_post_offices (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    location_id UUID NOT NULL REFERENCES locations(id) ON DELETE CASCADE,
    name VARCHAR(150) NOT NULL,
    branch_type VARCHAR(100) NOT NULL DEFAULT '',
    delivery_status VARCHAR(50) NOT NULL DEFAULT '',
    circle VARCHAR(100) NOT NULL DEFAULT '',
    district VARCHAR(100) NOT NULL DEFAULT '',
    division VARCHAR(100) NOT NULL DEFAULT '',
    region VARCHAR(100) NOT NULL DEFAULT '',
    state VARCHAR(100) NOT NULL DEFAULT '',
    country VARCHAR(100) NOT NULL DEFAULT '',
    pincode VARCHAR(10) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_location_post_offices_location_id ON location_post_offices(location_id);
CREATE INDEX IF NOT EXISTS idx_location_post_offices_pincode ON location_post_offices(pincode);
