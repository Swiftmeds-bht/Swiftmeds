CREATE TABLE IF NOT EXISTS prices (
    pharmacy_id     TEXT        NOT NULL,
    product_id      TEXT        NOT NULL,
    -- Money is always integer chhertum (100 = Nu. 1); the domain also rejects <= 0.
    amount_chhertum BIGINT      NOT NULL CHECK (amount_chhertum > 0),
    updated_at      TIMESTAMPTZ NOT NULL,
    PRIMARY KEY (pharmacy_id, product_id)
);

-- Comparison reads one product across many pharmacies, cheapest first.
CREATE INDEX IF NOT EXISTS prices_product_amount_idx ON prices (product_id, amount_chhertum);
