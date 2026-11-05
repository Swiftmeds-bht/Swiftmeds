CREATE TABLE IF NOT EXISTS pharmacies (
    pharmacy_id    TEXT PRIMARY KEY,
    owner_user_id  TEXT NOT NULL,
    name           TEXT NOT NULL,
    license_number TEXT NOT NULL UNIQUE,
    latitude       DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude      DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    hours          JSONB NOT NULL DEFAULT '[]'::jsonb,
    status         TEXT NOT NULL CHECK (status IN ('PENDING', 'VERIFIED', 'REJECTED', 'SUSPENDED')),
    status_reason  TEXT NOT NULL DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL,
    verified_at    TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS pharmacies_status_idx ON pharmacies (status);
