CREATE TABLE IF NOT EXISTS holds (
    hold_id           TEXT        PRIMARY KEY,
    user_id           TEXT        NOT NULL,
    pharmacy_id       TEXT        NOT NULL,
    product_id        TEXT        NOT NULL,
    -- Same rule as domain.NewHold: a hold reserves 1 to 5 packs.
    quantity          INTEGER     NOT NULL CHECK (quantity BETWEEN 1 AND 5),
    -- The six states of the hold state machine (domain/status.go).
    status            TEXT        NOT NULL
                                  CHECK (status IN ('PENDING', 'ACTIVE', 'COLLECTED', 'EXPIRED', 'CANCELLED', 'REJECTED')),
    -- Made when the hold becomes ACTIVE. Kept afterwards for the record, but only
    -- an ACTIVE hold can be collected with it.
    pickup_code       TEXT        NOT NULL DEFAULT '',
    created_at        TIMESTAMPTZ NOT NULL,
    -- NULL until the hold becomes ACTIVE.
    expires_at        TIMESTAMPTZ,
    status_changed_at TIMESTAMPTZ NOT NULL,
    status_reason     TEXT        NOT NULL DEFAULT '',
    -- An ACTIVE hold always has a pickup code and an expiry time.
    CONSTRAINT holds_active_has_code_and_expiry
        CHECK (status <> 'ACTIVE' OR (pickup_code <> '' AND expires_at IS NOT NULL))
);

-- A pickup code is unique among the ACTIVE holds of one pharmacy. Once a hold is
-- collected, cancelled or expired, its code can be used again.
CREATE UNIQUE INDEX IF NOT EXISTS holds_active_code_uidx
    ON holds (pharmacy_id, pickup_code)
    WHERE status = 'ACTIVE';

-- The open-hold limit counts one patient's PENDING and ACTIVE holds.
CREATE INDEX IF NOT EXISTS holds_user_open_idx
    ON holds (user_id)
    WHERE status IN ('PENDING', 'ACTIVE');

-- ListHolds pages newest first, by patient or by pharmacy.
CREATE INDEX IF NOT EXISTS holds_user_created_idx
    ON holds (user_id, created_at DESC, hold_id DESC);
CREATE INDEX IF NOT EXISTS holds_pharmacy_created_idx
    ON holds (pharmacy_id, created_at DESC, hold_id DESC);

-- The Expiry worker finds ACTIVE holds whose time has run out.
CREATE INDEX IF NOT EXISTS holds_active_expiry_idx
    ON holds (expires_at)
    WHERE status = 'ACTIVE';
