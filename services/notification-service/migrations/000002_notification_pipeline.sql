-- 000002_notification_pipeline.sql
-- Migration for Notification Event Processing Pipeline (NOTIF-01)

DO $$ BEGIN
    CREATE TYPE notification_job_status AS ENUM ('PENDING', 'PROCESSING', 'COMPLETED', 'FAILED', 'OPTED_OUT');
EXCEPTION
    WHEN duplicate_object THEN null;
END $$;

DO $$ BEGIN
    CREATE TYPE channel_type AS ENUM ('EMAIL', 'SMS', 'IN_APP', 'PUSH');
EXCEPTION
    WHEN duplicate_object THEN null;
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
    template_id UUID NOT NULL REFERENCES notification_templates(id) ON DELETE RESTRICT,
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
    ('SMS', 'SMS Notifications', TRUE),
    ('IN_APP', 'In-App Notifications', TRUE),
    ('PUSH', 'Push Notifications', TRUE)
ON CONFLICT (code) DO NOTHING;

-- Seed Templates for the 11 domain events explicitly defined by the SRS
INSERT INTO notification_templates (event_type, channel_id, subject_template, body_template, is_active)
SELECT 
    t.event_type,
    c.id,
    t.subject_template,
    t.body_template,
    TRUE
FROM (
    VALUES
        ('UserRegistered', 'Welcome to GoCart, {{.Email}}!', 'Hello {{.Email}}, your account has been registered successfully on GoCart. Welcome aboard!'),
        ('StoreApproved', 'Store Approved: Store #{{.StoreID}}', 'Congratulations! Your store application for Store #{{.StoreID}} has been approved. You may now start listing products.'),
        ('StoreRejected', 'Store Application Update: Store #{{.StoreID}}', 'Your store application for Store #{{.StoreID}} was rejected. Reason: {{.Reason}}. Please contact support if you have questions.'),
        ('OrderCreated', 'Order Confirmation - Order #{{.OrderID}}', 'Thank you for your order! Your order #{{.OrderID}} has been placed with total amount {{.TotalAmount}} {{.Currency}}.'),
        ('PaymentSuccessful', 'Payment Receipt - Order #{{.OrderID}}', 'Payment of {{.Amount}} {{.Currency}} for Order #{{.OrderID}} was successful. Reference Payment ID: {{.PaymentID}}.'),
        ('PaymentFailed', 'Payment Failed - Order #{{.OrderID}}', 'Payment of {{.Amount}} {{.Currency}} for Order #{{.OrderID}} failed. Reason: {{.Reason}}. Please update your payment method.'),
        ('OrderShipped', 'Your Order #{{.OrderID}} has Shipped!', 'Great news! Your order #{{.OrderID}} has been shipped via {{.DeliveryPartner}} with tracking number: {{.TrackingNumber}}.'),
        ('OrderDelivered', 'Order #{{.OrderID}} Delivered', 'Your order #{{.OrderID}} has been delivered successfully. Thank you for shopping with GoCart!'),
        ('ReturnApproved', 'Return Request Approved - Return #{{.ReturnID}}', 'Your return request for Order #{{.OrderID}} (Return ID: {{.ReturnID}}) has been approved. Refund amount: {{.RefundAmount}}.'),
        ('ReturnRejected', 'Return Request Rejected - Return #{{.ReturnID}}', 'Your return request for Order #{{.OrderID}} (Return ID: {{.ReturnID}}) has been rejected. Reason: {{.RejectionReason}}.'),
        ('RefundCompleted', 'Refund Completed - Order #{{.OrderID}}', 'Your refund of {{.Amount}} {{.Currency}} for Order #{{.OrderID}} (Refund ID: {{.RefundID}}) has been processed successfully.')
) AS t(event_type, subject_template, body_template)
CROSS JOIN (
    SELECT id FROM notification_channels WHERE code = 'EMAIL' LIMIT 1
) AS c
ON CONFLICT (event_type) DO UPDATE SET
    subject_template = EXCLUDED.subject_template,
    body_template = EXCLUDED.body_template;

