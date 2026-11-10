-- 000002_notification_pipeline.sql
-- Migration for Notification Event Processing Pipeline (NOTIF-01)

DO $$ BEGIN
    CREATE TYPE notification_job_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'OPTED_OUT');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE channel_type AS ENUM ('EMAIL', 'IN_APP', 'PUSH');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

-- Alter notification_channels to use explicit ENUM for code
ALTER TABLE notification_channels 
    ALTER COLUMN code TYPE channel_type USING code::channel_type;

-- Allow multiple channels per event type by scoping uniqueness to (event_type, channel_id)
ALTER TABLE notification_templates DROP CONSTRAINT IF EXISTS notification_templates_event_type_key;
DO $$ BEGIN
    IF NOT EXISTS (
        SELECT 1 FROM pg_constraint WHERE conname = 'uq_notification_templates_event_channel'
    ) THEN
        ALTER TABLE notification_templates ADD CONSTRAINT uq_notification_templates_event_channel UNIQUE (event_type, channel_id);
    END IF;
END $$;

-- User notification preferences (relational 3NF)
CREATE TABLE IF NOT EXISTS user_notification_preferences (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE CASCADE,
    event_type VARCHAR(100) NOT NULL,
    is_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    CONSTRAINT uq_user_pref UNIQUE (user_id, channel_id, event_type)
);

CREATE INDEX IF NOT EXISTS idx_user_notif_pref ON user_notification_preferences(user_id, event_type);

-- Processed events table for consumer idempotency
CREATE TABLE IF NOT EXISTS processed_events (
    event_id VARCHAR(255) PRIMARY KEY,
    event_type VARCHAR(100) NOT NULL,
    source VARCHAR(100) NOT NULL DEFAULT '',
    processed_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_processed_events_type ON processed_events(event_type);

-- Notification jobs table with recipient, channel, and template information
CREATE TABLE IF NOT EXISTS notification_jobs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    event_id VARCHAR(255) NOT NULL UNIQUE,
    user_id UUID,
    channel_id UUID NOT NULL REFERENCES notification_channels(id) ON DELETE RESTRICT,
    template_id UUID REFERENCES notification_templates(id) ON DELETE SET NULL,
    event_type VARCHAR(100) NOT NULL,
    recipient VARCHAR(255) NOT NULL,
    subject VARCHAR(255) NOT NULL,
    content TEXT NOT NULL,
    status notification_job_status NOT NULL DEFAULT 'PENDING',
    retry_count INT NOT NULL DEFAULT 0,
    max_retries INT NOT NULL DEFAULT 3,
    correlation_id VARCHAR(255),
    error_message TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    processed_at TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_notification_jobs_status ON notification_jobs(status);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_user_id ON notification_jobs(user_id);
CREATE INDEX IF NOT EXISTS idx_notification_jobs_created_at ON notification_jobs(created_at);

-- Seed Channels
INSERT INTO notification_channels (code, name, is_active) VALUES
    ('EMAIL', 'Email Notifications', TRUE),
    ('IN_APP', 'In-App Notifications', TRUE),
    ('PUSH', 'Push Notifications', TRUE)
ON CONFLICT (code) DO NOTHING;

