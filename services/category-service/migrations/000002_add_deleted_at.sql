ALTER TABLE categories
    ADD COLUMN IF NOT EXISTS deleted_at TIMESTAMPTZ;

CREATE INDEX IF NOT EXISTS idx_categories_deleted_at ON categories(deleted_at);

DROP INDEX IF EXISTS uq_categories_parent_name;
DROP INDEX IF EXISTS uq_categories_root_name;

CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_parent_name 
    ON categories (parent_category_id, LOWER(name)) 
    WHERE parent_category_id IS NOT NULL AND deleted_at IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_categories_root_name 
    ON categories (LOWER(name)) 
    WHERE parent_category_id IS NULL AND deleted_at IS NULL;
