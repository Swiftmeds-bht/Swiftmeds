CREATE TABLE IF NOT EXISTS inventory_stock (
    pharmacy_id          TEXT        NOT NULL,
    product_id           TEXT        NOT NULL,
    quantity_on_hand     INTEGER     NOT NULL DEFAULT 0 CHECK (quantity_on_hand >= 0),
    quantity_reserved    INTEGER     NOT NULL DEFAULT 0 CHECK (
        quantity_reserved >= 0 AND quantity_reserved <= quantity_on_hand
    ),
    confirmed_at         TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_by_user_id   TEXT        NOT NULL DEFAULT '',
    PRIMARY KEY (pharmacy_id, product_id)
);

CREATE INDEX IF NOT EXISTS inventory_stock_product_pharmacy_idx
    ON inventory_stock (product_id, pharmacy_id);

-- Keep even unavailable hold requests so retries for the same hold_id stay idempotent.
-- No foreign key to inventory_stock: a pharmacy/product may have no stock row.
CREATE TABLE IF NOT EXISTS inventory_reservations (
    hold_id      TEXT        PRIMARY KEY,
    pharmacy_id  TEXT        NOT NULL,
    product_id   TEXT        NOT NULL,
    quantity     INTEGER     NOT NULL CHECK (quantity > 0),
    status       TEXT        NOT NULL CHECK (
        status IN ('RESERVED', 'COMMITTED', 'RELEASED', 'UNAVAILABLE')
    ),
    created_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX IF NOT EXISTS inventory_reservations_stock_idx
    ON inventory_reservations (pharmacy_id, product_id);