-- 000003_remove_push_channel.sql
-- Remove PUSH and IN_APP notification channels, keeping EMAIL alone

DELETE FROM user_notification_preferences WHERE channel_id IN (SELECT id FROM notification_channels WHERE code::text IN ('PUSH', 'IN_APP'));
DELETE FROM notification_jobs WHERE channel_id IN (SELECT id FROM notification_channels WHERE code::text IN ('PUSH', 'IN_APP'));
DELETE FROM notification_templates WHERE channel_id IN (SELECT id FROM notification_channels WHERE code::text IN ('PUSH', 'IN_APP'));
DELETE FROM notifications WHERE channel_id IN (SELECT id FROM notification_channels WHERE code::text IN ('PUSH', 'IN_APP'));
DELETE FROM notification_channels WHERE code::text IN ('PUSH', 'IN_APP');

