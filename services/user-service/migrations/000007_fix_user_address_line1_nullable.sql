ALTER TABLE user_addresses ALTER COLUMN address_line1 DROP NOT NULL;
ALTER TABLE user_addresses ALTER COLUMN address_line1 SET DEFAULT '';
UPDATE user_addresses SET address_line = address_line1 WHERE address_line IS NULL AND address_line1 IS NOT NULL;
UPDATE user_addresses SET address_line1 = address_line WHERE address_line1 IS NULL AND address_line IS NOT NULL;
