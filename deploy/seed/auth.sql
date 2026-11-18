-- Demo accounts (SCRUM-66), so search -> reserve -> collect can be tried right after `make up`.
--
-- Every account's password is: swiftmeds-demo
-- These are development fixtures only. Never load this file into a real environment.
--
-- * One demo patient: patient@demo.swiftmeds.test
-- * One demo admin: admin@demo.swiftmeds.test (SCRUM-75), for the admin-only
--   routes; like a pharmacist, it can also use the pharmacy routes.
-- * One demo pharmacist per pharmacy in pharmacies.csv, signed in as the
--   pharmacy's license number, for example demo-tim-001@demo.swiftmeds.test.
--   Registering through the API never links a pharmacist to a pharmacy, and the
--   gateway needs that link (the token's pharmacy_id) to collect a hold, so the
--   link is made here. The user ID is the pharmacy's owner_user_id from the CSV.
--
-- Re-running is safe: existing demo rows are updated, nothing else is touched.

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

-- bcrypt (cost 10) of "swiftmeds-demo".
INSERT INTO users (id, email, password_hash, role, pharmacy_id, created_at)
VALUES (
    '31000000-0000-4000-8000-000000000001',
    'patient@demo.swiftmeds.test',
    '$2a$10$PsbBqtED/0FLqhyzF/Cmkucj5CZlXm1AmF2clpxytXZ4/.JcDdL8K',
    'PATIENT',
    NULL,
    now()
)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    role = EXCLUDED.role,
    pharmacy_id = EXCLUDED.pharmacy_id;

INSERT INTO users (id, email, password_hash, role, pharmacy_id, created_at)
VALUES (
    '32000000-0000-4000-8000-000000000001',
    'admin@demo.swiftmeds.test',
    '$2a$10$PsbBqtED/0FLqhyzF/Cmkucj5CZlXm1AmF2clpxytXZ4/.JcDdL8K',
    'ADMIN',
    NULL,
    now()
)
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    role = EXCLUDED.role,
    pharmacy_id = EXCLUDED.pharmacy_id;

INSERT INTO users (id, email, password_hash, role, pharmacy_id, created_at)
SELECT
    owner_user_id,
    lower(license_number) || '@demo.swiftmeds.test',
    '$2a$10$PsbBqtED/0FLqhyzF/Cmkucj5CZlXm1AmF2clpxytXZ4/.JcDdL8K',
    'PHARMACIST',
    pharmacy_id,
    now()
FROM seed_pharmacies
ON CONFLICT (email) DO UPDATE
SET password_hash = EXCLUDED.password_hash,
    role = EXCLUDED.role,
    pharmacy_id = EXCLUDED.pharmacy_id;

COMMIT;
