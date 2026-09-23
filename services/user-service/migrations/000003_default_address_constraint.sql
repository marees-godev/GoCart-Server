WITH ranked_defaults AS (
    SELECT id, ROW_NUMBER() OVER (PARTITION BY user_id ORDER BY updated_at DESC, created_at DESC) AS rn
    FROM user_addresses
    WHERE is_default = TRUE
)
UPDATE user_addresses
SET is_default = FALSE
WHERE id IN (
    SELECT id FROM ranked_defaults WHERE rn > 1
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_user_addresses_unique_default_per_user
ON user_addresses (user_id)
WHERE is_default = TRUE;
