-- Add warehouse_id foreign key column to inventories table
ALTER TABLE inventories ADD COLUMN IF NOT EXISTS warehouse_id UUID REFERENCES warehouses(id) ON DELETE SET NULL;
CREATE INDEX IF NOT EXISTS idx_inventories_warehouse_id ON inventories(warehouse_id);
