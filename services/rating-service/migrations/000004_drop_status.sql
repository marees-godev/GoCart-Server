DROP INDEX IF EXISTS idx_ratings_status;
ALTER TABLE ratings DROP COLUMN IF EXISTS status;

