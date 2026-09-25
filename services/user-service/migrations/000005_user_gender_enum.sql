DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_gender') THEN 
        CREATE TYPE user_gender AS ENUM ('male', 'female', 'others'); 
    END IF; 
END $$;

ALTER TABLE users 
    ALTER COLUMN gender TYPE user_gender USING (
        CASE 
            WHEN LOWER(TRIM(gender)) = 'male' THEN 'male'::user_gender
            WHEN LOWER(TRIM(gender)) = 'female' THEN 'female'::user_gender
            WHEN LOWER(TRIM(gender)) = 'others' THEN 'others'::user_gender
            ELSE NULL
        END
    );
