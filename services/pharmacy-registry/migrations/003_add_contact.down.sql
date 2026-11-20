DROP INDEX IF EXISTS pharmacies_owner_idx;
ALTER TABLE pharmacies
    DROP COLUMN IF EXISTS address,
    DROP COLUMN IF EXISTS phone;
