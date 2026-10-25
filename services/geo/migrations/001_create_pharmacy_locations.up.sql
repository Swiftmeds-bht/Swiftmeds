-- Locations of verified pharmacies (a copy kept in sync by Pharmacy Registry).
CREATE TABLE IF NOT EXISTS pharmacy_locations (
    pharmacy_id TEXT             PRIMARY KEY,
    latitude    DOUBLE PRECISION NOT NULL CHECK (latitude BETWEEN -90 AND 90),
    longitude   DOUBLE PRECISION NOT NULL CHECK (longitude BETWEEN -180 AND 180),
    updated_at  TIMESTAMPTZ      NOT NULL DEFAULT now()
);

-- Bounding-box pre-filter: latitude range first, then longitude.
CREATE INDEX IF NOT EXISTS pharmacy_locations_lat_lng_idx ON pharmacy_locations (latitude, longitude);
