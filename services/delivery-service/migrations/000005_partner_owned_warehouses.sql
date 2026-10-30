-- Drop the partner_warehouses association table
DROP TABLE IF EXISTS public.partner_warehouses CASCADE;

-- Add delivery_partner_id column to warehouses table
ALTER TABLE public.warehouses ADD COLUMN IF NOT EXISTS delivery_partner_id UUID;

-- Create index on delivery_partner_id
CREATE INDEX IF NOT EXISTS idx_warehouses_delivery_partner_id ON public.warehouses(delivery_partner_id);

-- Drop global unique constraint on code if it exists
ALTER TABLE public.warehouses DROP CONSTRAINT IF EXISTS warehouses_code_key;

-- Ensure code is unique per delivery partner
CREATE UNIQUE INDEX IF NOT EXISTS idx_warehouses_partner_code ON public.warehouses(delivery_partner_id, code);
