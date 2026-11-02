CREATE EXTENSION IF NOT EXISTS pg_trgm;

CREATE TABLE IF NOT EXISTS catalog_molecules (
    molecule_id   TEXT PRIMARY KEY,
    molecule_name TEXT NOT NULL CHECK (btrim(molecule_name) <> '')
);

CREATE TABLE IF NOT EXISTS catalog_products (
    product_id            TEXT PRIMARY KEY,
    brand_name            TEXT NOT NULL CHECK (btrim(brand_name) <> ''),
    generic_name          TEXT NOT NULL CHECK (btrim(generic_name) <> ''),
    molecule_id           TEXT NOT NULL REFERENCES catalog_molecules (molecule_id),
    strength              TEXT NOT NULL CHECK (btrim(strength) <> ''),
    dosage_form           TEXT NOT NULL CHECK (btrim(dosage_form) <> ''),
    pack_size             INTEGER NOT NULL CHECK (pack_size > 0),
    manufacturer          TEXT NOT NULL DEFAULT '',
    requires_prescription BOOLEAN NOT NULL DEFAULT FALSE,
    controlled            BOOLEAN NOT NULL DEFAULT FALSE
);

CREATE INDEX IF NOT EXISTS catalog_products_brand_prefix_idx
    ON catalog_products (lower(brand_name) text_pattern_ops);

CREATE INDEX IF NOT EXISTS catalog_products_generic_prefix_idx
    ON catalog_products (lower(generic_name) text_pattern_ops);

CREATE INDEX IF NOT EXISTS catalog_products_brand_trgm_idx
    ON catalog_products USING GIN (lower(brand_name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS catalog_products_generic_trgm_idx
    ON catalog_products USING GIN (lower(generic_name) gin_trgm_ops);

CREATE INDEX IF NOT EXISTS catalog_products_molecule_strength_idx
    ON catalog_products (molecule_id, strength);

CREATE INDEX IF NOT EXISTS catalog_molecules_name_trgm_idx
    ON catalog_molecules USING GIN (lower(molecule_name) gin_trgm_ops);