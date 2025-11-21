DROP INDEX IF EXISTS idx_is_deleted;

ALTER TABLE urls DROP COLUMN IF EXISTS is_deleted;

