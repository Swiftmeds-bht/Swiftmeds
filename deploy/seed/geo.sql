BEGIN;

CREATE TEMP TABLE seed_pharmacies (
    pharmacy_id TEXT,
    owner_user_id TEXT,
    name TEXT,
    license_number TEXT,
    phone TEXT,
    address TEXT,
    dzongkhag TEXT,
    latitude DOUBLE PRECISION,
    longitude DOUBLE PRECISION,
    status TEXT
) ON COMMIT DROP;

\copy seed_pharmacies FROM '/opt/swiftmed/seed/pharmacies.csv' WITH (FORMAT csv, HEADER true)

INSERT INTO pharmacy_locations (pharmacy_id, latitude, longitude, updated_at)
SELECT pharmacy_id, latitude, longitude, CURRENT_TIMESTAMP
FROM seed_pharmacies
WHERE status = 'VERIFICATION_STATUS_VERIFIED'
ON CONFLICT (pharmacy_id) DO UPDATE SET
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    updated_at = EXCLUDED.updated_at;

COMMIT;
