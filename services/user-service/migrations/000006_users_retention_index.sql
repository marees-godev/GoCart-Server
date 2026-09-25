CREATE INDEX IF NOT EXISTS idx_users_retention
    ON users (deactivated_at ASC)
    WHERE status = 'deactivated';
