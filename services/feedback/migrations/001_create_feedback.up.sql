CREATE TABLE IF NOT EXISTS feedback (
    feedback_id  TEXT        PRIMARY KEY,
    hold_id      TEXT        NOT NULL,
    -- Copied from the hold when the feedback is made, so a pharmacy's feedback
    -- can be listed without asking Holds.
    pharmacy_id  TEXT        NOT NULL,
    product_id   TEXT        NOT NULL,
    was_in_stock BOOLEAN     NOT NULL,
    -- 1 to 5, or 0 when the patient gave no rating. Same rule as domain.NewFeedback.
    rating       INTEGER     NOT NULL DEFAULT 0 CHECK (rating BETWEEN 0 AND 5),
    -- At most 500 characters, as in feedback.proto.
    comment      TEXT        NOT NULL DEFAULT '' CHECK (char_length(comment) <= 500),
    created_at   TIMESTAMPTZ NOT NULL
);

-- One feedback per hold. This index, not the app, is what stops two submissions
-- that arrive at the same moment from both being stored.
CREATE UNIQUE INDEX IF NOT EXISTS feedback_hold_uidx
    ON feedback (hold_id);

-- ListPharmacyFeedback pages newest first.
CREATE INDEX IF NOT EXISTS feedback_pharmacy_created_idx
    ON feedback (pharmacy_id, created_at DESC, feedback_id DESC);
