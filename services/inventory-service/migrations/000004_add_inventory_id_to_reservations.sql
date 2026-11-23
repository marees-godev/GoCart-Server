ALTER TABLE inventory_reservations ADD COLUMN IF NOT EXISTS inventory_id UUID REFERENCES inventories(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_inv_res_inventory_id ON inventory_reservations(inventory_id);
