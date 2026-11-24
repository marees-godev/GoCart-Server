-- 000004_category_preferences.sql
-- Migration to switch user preferences from micro-events to configurable categories

DO $$ BEGIN
    CREATE TYPE notification_category AS ENUM (
        'ORDERS',
        'DELIVERIES',
        'RETURNS',
        'ACCOUNT'
    );
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Drop old event-based constraint & index
ALTER TABLE user_notification_preferences DROP CONSTRAINT IF EXISTS uq_user_pref;
DROP INDEX IF EXISTS idx_user_notif_pref;

-- Drop event_type and add category ENUM column
ALTER TABLE user_notification_preferences DROP COLUMN IF EXISTS event_type;
ALTER TABLE user_notification_preferences 
    ADD COLUMN IF NOT EXISTS category notification_category NOT NULL;

-- 3NF unique constraint: only 1 row per user per channel per category
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_user_pref_category'
    ) THEN
        ALTER TABLE user_notification_preferences 
            ADD CONSTRAINT uq_user_pref_category UNIQUE (user_id, channel_id, category);
    END IF;
END $$;

CREATE INDEX IF NOT EXISTS idx_user_notif_pref_category 
    ON user_notification_preferences(user_id, category);
