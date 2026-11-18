-- SCRUM-125: lock a hold after too many wrong pickup codes, and recover holds
-- whose reservation events were lost.

-- Wrong pickup codes tried for the hold. At domain.MaxPickupAttempts the hold
-- is locked. Only ever increased with one UPDATE, so concurrent tries all count.
ALTER TABLE holds ADD COLUMN IF NOT EXISTS pickup_attempts INTEGER NOT NULL DEFAULT 0
    CHECK (pickup_attempts >= 0);

-- Set when the sweep rejected a stale PENDING hold for which Inventory may still
-- hold stock; cleared once Inventory released it.
ALTER TABLE holds ADD COLUMN IF NOT EXISTS stock_release_pending BOOLEAN NOT NULL DEFAULT FALSE;

-- The sweep finds PENDING holds older than the timeout, oldest first ...
CREATE INDEX IF NOT EXISTS holds_pending_created_idx
    ON holds (created_at, hold_id)
    WHERE status = 'PENDING';

-- ... and the timed-out holds whose stock is still to be released.
CREATE INDEX IF NOT EXISTS holds_release_pending_idx
    ON holds (status_changed_at, hold_id)
    WHERE stock_release_pending;
