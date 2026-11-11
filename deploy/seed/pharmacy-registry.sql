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

INSERT INTO pharmacies (
    pharmacy_id,
    owner_user_id,
    name,
    license_number,
    latitude,
    longitude,
    status,
    created_at,
    verified_at
)
SELECT
    pharmacy_id,
    owner_user_id,
    name,
    license_number,
    latitude,
    longitude,
    CASE status
        WHEN 'VERIFICATION_STATUS_VERIFIED' THEN 'VERIFIED'
        ELSE NULL
    END,
    CURRENT_TIMESTAMP,
    CURRENT_TIMESTAMP
FROM seed_pharmacies
ON CONFLICT (pharmacy_id) DO UPDATE SET
    owner_user_id = EXCLUDED.owner_user_id,
    name = EXCLUDED.name,
    license_number = EXCLUDED.license_number,
    latitude = EXCLUDED.latitude,
    longitude = EXCLUDED.longitude,
    status = EXCLUDED.status,
    verified_at = EXCLUDED.verified_at;

COMMIT;
