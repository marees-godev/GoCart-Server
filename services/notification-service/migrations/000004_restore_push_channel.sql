INSERT INTO notification_channels (code, name, is_active)
VALUES ('PUSH'::channel_type, 'Push Notifications', TRUE)
ON CONFLICT (code) DO UPDATE
SET name = EXCLUDED.name,
    is_active = TRUE;

