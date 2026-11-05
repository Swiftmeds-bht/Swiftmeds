CREATE TABLE IF NOT EXISTS subscriptions (
    subscription_id TEXT             PRIMARY KEY,
    user_id         TEXT             NOT NULL,
    product_id      TEXT             NOT NULL,
    latitude        DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude       DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    radius_m        INTEGER          NOT NULL CHECK (radius_m BETWEEN 1 AND 50000),
    created_at      TIMESTAMPTZ      NOT NULL,
    -- Set when the alert fires; a notified subscription never fires again.
    notified_at     TIMESTAMPTZ
);

-- One waiting subscription per user and product: subscribing twice returns the first.
CREATE UNIQUE INDEX IF NOT EXISTS subscriptions_active_user_product_idx
    ON subscriptions (user_id, product_id) WHERE notified_at IS NULL;

-- A stock.increased event looks up the waiting subscribers of one product.
CREATE INDEX IF NOT EXISTS subscriptions_active_product_idx
    ON subscriptions (product_id) WHERE notified_at IS NULL;

CREATE INDEX IF NOT EXISTS subscriptions_user_created_idx
    ON subscriptions (user_id, created_at DESC);

-- Back-in-stock alerts waiting to be delivered by StreamAlerts (SCRUM-71).
CREATE TABLE IF NOT EXISTS alerts (
    alert_id           TEXT        PRIMARY KEY,
    subscription_id    TEXT        NOT NULL REFERENCES subscriptions (subscription_id) ON DELETE CASCADE,
    user_id            TEXT        NOT NULL,
    product_id         TEXT        NOT NULL,
    pharmacy_id        TEXT        NOT NULL,
    quantity_available INTEGER     NOT NULL,
    occurred_at        TIMESTAMPTZ NOT NULL,
    delivered_at       TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS alerts_undelivered_user_idx
    ON alerts (user_id, occurred_at) WHERE delivered_at IS NULL;
