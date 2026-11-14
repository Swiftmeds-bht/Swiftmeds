CREATE TABLE analytics_events (
    event_id TEXT PRIMARY KEY,
    event_type TEXT NOT NULL CHECK (event_type IN ('SEARCH_RECORDED', 'STOCK_UNAVAILABLE')),
    search_id TEXT NOT NULL DEFAULT '',
    pharmacy_id TEXT NOT NULL DEFAULT '',
    product_id TEXT NOT NULL,
    product_name TEXT NOT NULL,
    dzongkhag TEXT NOT NULL DEFAULT '',
    occurred_at TIMESTAMPTZ NOT NULL,
    has_stock BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX analytics_events_demand_idx
    ON analytics_events (occurred_at, dzongkhag, product_id);
