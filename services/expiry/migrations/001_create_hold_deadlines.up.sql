-- One row per confirmed hold: when it runs out, and how that deadline was settled.
CREATE TABLE IF NOT EXISTS hold_deadlines (
    hold_id      TEXT        PRIMARY KEY,
    expires_at   TIMESTAMPTZ NOT NULL,
    received_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- NULL while the deadline is pending. Set once, by the sweep that settles it.
    completed_at TIMESTAMPTZ,
    outcome      TEXT        NOT NULL DEFAULT ''
                             CHECK (outcome IN ('', 'EXPIRED', 'ALREADY_FINAL', 'NOT_FOUND')),
    -- A deadline has an outcome exactly when it is settled.
    CONSTRAINT hold_deadlines_outcome_matches_completion
        CHECK ((completed_at IS NULL) = (outcome = ''))
);

-- The sweep reads pending deadlines in (expires_at, hold_id) order.
CREATE INDEX IF NOT EXISTS hold_deadlines_pending_idx
    ON hold_deadlines (expires_at, hold_id)
    WHERE completed_at IS NULL;
