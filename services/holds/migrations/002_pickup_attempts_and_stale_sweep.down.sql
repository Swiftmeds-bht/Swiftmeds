DROP INDEX IF EXISTS holds_release_pending_idx;
DROP INDEX IF EXISTS holds_pending_created_idx;
ALTER TABLE holds DROP COLUMN IF EXISTS stock_release_pending;
ALTER TABLE holds DROP COLUMN IF EXISTS pickup_attempts;
