DO $$ BEGIN
    CREATE TYPE auth_role AS ENUM (
        'USER',
        'MERCHANT',
        'DELIVERY_PARTNER',
        'ADMIN'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- 1. If is_merchant column exists, migrate data
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'auth_credentials' AND column_name = 'is_merchant'
    ) THEN
        UPDATE auth_credentials
        SET role = CASE 
            WHEN is_merchant = true THEN 'MERCHANT'
            ELSE 'USER'
        END;
        ALTER TABLE auth_credentials DROP COLUMN is_merchant;
    END IF;
END $$;

-- 2. Migrate existing role strings if needed (e.g. 'customer' -> 'USER', 'CUSTOMER' -> 'USER', 'merchant' -> 'MERCHANT')
UPDATE auth_credentials
SET role = CASE
    WHEN UPPER(role) = 'ADMIN' THEN 'ADMIN'
    WHEN UPPER(role) = 'MERCHANT' THEN 'MERCHANT'
    WHEN UPPER(role) = 'DELIVERY_PARTNER' THEN 'DELIVERY_PARTNER'
    ELSE 'USER'
END
WHERE role IS NOT NULL;

-- 3. Alter role column to auth_role enum
DO $$ BEGIN
    IF EXISTS (
        SELECT 1 FROM information_schema.columns 
        WHERE table_name = 'auth_credentials' AND column_name = 'role' AND data_type = 'character varying'
    ) THEN
        ALTER TABLE auth_credentials ALTER COLUMN role DROP DEFAULT;
        ALTER TABLE auth_credentials ALTER COLUMN role TYPE auth_role USING (role::auth_role);
        ALTER TABLE auth_credentials ALTER COLUMN role SET DEFAULT 'USER'::auth_role;
    END IF;
END $$;

-- 4. Enforce account constraint on (email, role)
ALTER TABLE auth_credentials DROP CONSTRAINT IF EXISTS auth_credentials_email_key;
DROP INDEX IF EXISTS idx_auth_credentials_email;
DROP INDEX IF EXISTS idx_auth_credentials_email_role;
CREATE UNIQUE INDEX IF NOT EXISTS idx_auth_credentials_email_role ON auth_credentials (LOWER(email), role);
