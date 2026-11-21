-- Phone and address, for the administrator who verifies a new pharmacy (SCRUM-141).
ALTER TABLE pharmacies
    ADD COLUMN IF NOT EXISTS phone TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS address TEXT NOT NULL DEFAULT '';

-- Looked up when a pharmacy registers, so a repeated registration returns the same pharmacy.
CREATE INDEX IF NOT EXISTS pharmacies_owner_idx ON pharmacies (owner_user_id);
