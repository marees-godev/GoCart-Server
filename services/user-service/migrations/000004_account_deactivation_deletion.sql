DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_status') THEN 
        CREATE TYPE user_status AS ENUM ('active', 'deactivated', 'deleted', 'suspended'); 
    END IF; 
END $$;

ALTER TABLE users 
    ALTER COLUMN status DROP DEFAULT,
    ALTER COLUMN status TYPE user_status USING status::user_status,
    ALTER COLUMN status SET DEFAULT 'active'::user_status;

ALTER TABLE users 
    ADD COLUMN IF NOT EXISTS deactivated_at TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_users_deactivated_at
    ON users (deactivated_at)
    WHERE status = 'deactivated';

DO $$ 
BEGIN 
    IF NOT EXISTS (SELECT 1 FROM pg_type WHERE typname = 'user_audit_action') THEN 
        CREATE TYPE user_audit_action AS ENUM ('DEACTIVATE', 'DELETE', 'REACTIVATE'); 
    END IF; 
END $$;

CREATE TABLE IF NOT EXISTS user_audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE RESTRICT,
    action user_audit_action NOT NULL,
    performed_by UUID NOT NULL,
    reason TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_user_audit_logs_user_id ON user_audit_logs(user_id);
CREATE INDEX IF NOT EXISTS idx_user_audit_logs_created_at ON user_audit_logs(created_at);

ALTER TABLE outbox_events
    ADD COLUMN IF NOT EXISTS topic VARCHAR(255) NOT NULL DEFAULT '';

CREATE INDEX IF NOT EXISTS idx_outbox_pending_created
    ON outbox_events (created_at ASC)
    WHERE status = 'PENDING';
