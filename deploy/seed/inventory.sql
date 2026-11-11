BEGIN;

CREATE TEMP TABLE seed_stock (
    pharmacy_id TEXT,
    product_id TEXT,
    quantity_on_hand INTEGER
) ON COMMIT DROP;

\copy seed_stock FROM '/opt/swiftmed/seed/stock.csv' WITH (FORMAT csv, HEADER true)

INSERT INTO inventory_stock (
    pharmacy_id,
    product_id,
    quantity_on_hand,
    quantity_reserved,
    confirmed_at,
    updated_by_user_id
)
SELECT
    pharmacy_id,
    product_id,
    quantity_on_hand,
    0,
    CURRENT_TIMESTAMP,
    'demo-seed'
FROM seed_stock
ON CONFLICT (pharmacy_id, product_id) DO UPDATE SET
    quantity_on_hand = GREATEST(EXCLUDED.quantity_on_hand, inventory_stock.quantity_reserved),
    confirmed_at = EXCLUDED.confirmed_at,
    updated_by_user_id = EXCLUDED.updated_by_user_id;

COMMIT;
