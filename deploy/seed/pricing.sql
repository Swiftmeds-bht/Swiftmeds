BEGIN;

CREATE TEMP TABLE seed_prices (
    pharmacy_id TEXT,
    product_id TEXT,
    amount_chhertum BIGINT
) ON COMMIT DROP;

\copy seed_prices FROM '/opt/swiftmed/seed/prices.demo.csv' WITH (FORMAT csv, HEADER true)

INSERT INTO prices (pharmacy_id, product_id, amount_chhertum, updated_at)
SELECT pharmacy_id, product_id, amount_chhertum, CURRENT_TIMESTAMP
FROM seed_prices
ON CONFLICT (pharmacy_id, product_id) DO UPDATE SET
    amount_chhertum = EXCLUDED.amount_chhertum,
    updated_at = EXCLUDED.updated_at;

COMMIT;
