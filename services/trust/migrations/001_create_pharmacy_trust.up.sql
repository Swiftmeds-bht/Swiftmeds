-- One row per pharmacy that has feedback. A pharmacy without a row is neutral (0.5).
-- The score itself is not stored: it depends on the time it is asked for
-- (old feedback fades), so GetTrust computes it from these sums.
CREATE TABLE IF NOT EXISTS pharmacy_trust (
    pharmacy_id       TEXT             PRIMARY KEY,
    feedback_count    INTEGER          NOT NULL DEFAULT 0 CHECK (feedback_count >= 0),
    in_stock_count    INTEGER          NOT NULL DEFAULT 0 CHECK (in_stock_count BETWEEN 0 AND feedback_count),
    -- Σ w·x and Σ w over all feedback, each weight taken at last_feedback_at.
    weighted_in_stock DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (weighted_in_stock >= 0),
    weighted_total    DOUBLE PRECISION NOT NULL DEFAULT 0 CHECK (weighted_total >= 0),
    -- When the newest feedback was given (the event's occurred_at).
    last_feedback_at  TIMESTAMPTZ,
    updated_at        TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- Every feedback_id already counted, so the same feedback never counts twice,
-- even if Feedback publishes it again under a new event ID.
CREATE TABLE IF NOT EXISTS applied_feedback (
    feedback_id TEXT        PRIMARY KEY,
    pharmacy_id TEXT        NOT NULL,
    applied_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);
