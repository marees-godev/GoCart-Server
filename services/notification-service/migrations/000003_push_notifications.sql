DO $$ BEGIN
    CREATE TYPE notification_delivery_status AS ENUM ('PENDING', 'SENT', 'FAILED');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    IF EXISTS (
        SELECT 1
        FROM information_schema.columns
        WHERE table_name = 'notifications'
          AND column_name = 'status'
          AND udt_name <> 'notification_delivery_status'
    ) THEN
        UPDATE notifications
        SET status = 'PENDING'
        WHERE status IS NULL OR status NOT IN ('PENDING', 'SENT', 'FAILED');

        ALTER TABLE notifications ALTER COLUMN status DROP DEFAULT;
        ALTER TABLE notifications
            ALTER COLUMN status TYPE notification_delivery_status
            USING status::notification_delivery_status;
        ALTER TABLE notifications
            ALTER COLUMN status SET DEFAULT 'PENDING'::notification_delivery_status;
    END IF;
END $$;

ALTER TABLE notifications
    ADD COLUMN IF NOT EXISTS event_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS correlation_id VARCHAR(255),
    ADD COLUMN IF NOT EXISTS retry_count INT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS read_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW();

CREATE UNIQUE INDEX IF NOT EXISTS uq_notifications_event_channel
    ON notifications(event_id, channel_id)
    WHERE event_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_notifications_user_created
    ON notifications(user_id, created_at DESC);

CREATE INDEX IF NOT EXISTS idx_notifications_user_read
    ON notifications(user_id, is_read);

CREATE INDEX IF NOT EXISTS idx_notifications_event_type
    ON notifications(event_type);

CREATE TABLE IF NOT EXISTS push_subscriptions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    endpoint TEXT NOT NULL UNIQUE,
    p256dh TEXT NOT NULL,
    auth TEXT NOT NULL,
    user_agent TEXT NOT NULL DEFAULT '',
    device_name VARCHAR(255) NOT NULL DEFAULT '',
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    failure_count INT NOT NULL DEFAULT 0,
    last_error TEXT,
    last_used_at TIMESTAMPTZ,
    deactivated_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_push_subscriptions_user_active
    ON push_subscriptions(user_id, is_active);

CREATE INDEX IF NOT EXISTS idx_push_subscriptions_created_at
    ON push_subscriptions(created_at DESC);

