-- Freshness service: when each stock row was last confirmed, and its confidence.
-- Safe to run more than once: the service runs every *.up.sql file when it starts.
CREATE TABLE IF NOT EXISTS stock_confirmation (
    pharmacy_id  TEXT        NOT NULL,
    product_id   TEXT        NOT NULL,
    confirmed_at TIMESTAMPTZ NOT NULL,
    confidence   TEXT        NOT NULL CHECK (confidence IN ('HIGH', 'MEDIUM', 'LOW')),
    PRIMARY KEY (pharmacy_id, product_id)
);
