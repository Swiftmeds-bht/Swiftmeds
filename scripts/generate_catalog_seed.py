import csv
from pathlib import Path

ROOT = Path(__file__).resolve().parents[1]
SOURCE = ROOT / "deploy/seed/medicines.csv"
MIGRATIONS = ROOT / "services/catalog/migrations"

EXPECTED_FIELDS = [
    "product_id",
    "brand_name",
    "generic_name",
    "molecule_id",
    "molecule_name",
    "strength",
    "dosage_form",
    "pack_size",
    "manufacturer",
    "requires_prescription",
    "controlled",
]

DOSAGE_FORMS = {
    "DOSAGE_FORM_TABLET",
    "DOSAGE_FORM_CAPSULE",
    "DOSAGE_FORM_SYRUP",
    "DOSAGE_FORM_SUSPENSION",
    "DOSAGE_FORM_INJECTION",
    "DOSAGE_FORM_CREAM",
    "DOSAGE_FORM_OINTMENT",
    "DOSAGE_FORM_DROPS",
    "DOSAGE_FORM_INHALER",
    "DOSAGE_FORM_OTHER",
}


def sql_text(value):
    return "'" + value.replace("'", "''") + "'"


def sql_bool(value):
    normalized = value.strip().lower()
    if normalized not in {"true", "false"}:
        raise ValueError(f"invalid boolean value: {value!r}")
    return normalized.upper()


with SOURCE.open(newline="", encoding="utf-8") as file:
    reader = csv.DictReader(file)
    if reader.fieldnames != EXPECTED_FIELDS:
        raise ValueError(f"unexpected CSV header: {reader.fieldnames}")
    rows = list(reader)

if not rows:
    raise ValueError("medicines.csv contains no data rows")

molecules = {}
product_ids = set()
for row in rows:
    product_id = row["product_id"].strip()
    molecule_id = row["molecule_id"].strip()
    molecule_name = row["molecule_name"].strip()
    strength = "".join(row["strength"].split()).lower()
    dosage_form = row["dosage_form"].strip()
    pack_size = int(row["pack_size"])

    if not product_id or not molecule_id or not molecule_name:
        raise ValueError(f"missing ID or molecule name in row: {row}")
    if product_id in product_ids:
        raise ValueError(f"duplicate product ID: {product_id}")
    if dosage_form not in DOSAGE_FORMS:
        raise ValueError(f"invalid dosage form: {dosage_form}")
    if not strength or pack_size <= 0:
        raise ValueError(f"invalid strength or pack size in row: {row}")

    product_ids.add(product_id)
    previous_name = molecules.setdefault(molecule_id, molecule_name)
    if previous_name != molecule_name:
        raise ValueError(f"conflicting names for molecule {molecule_id}")

molecule_values = ",\n".join(
    f"    ({sql_text(molecule_id)}, {sql_text(molecule_name)})"
    for molecule_id, molecule_name in molecules.items()
)

product_values = []
for row in rows:
    product_values.append(
        "    ("
        + ", ".join(
            [
                sql_text(row["product_id"].strip()),
                sql_text(row["brand_name"].strip()),
                sql_text(row["generic_name"].strip()),
                sql_text(row["molecule_id"].strip()),
                sql_text("".join(row["strength"].split()).lower()),
                sql_text(row["dosage_form"].strip()),
                str(int(row["pack_size"])),
                sql_text(row["manufacturer"].strip()),
                sql_bool(row["requires_prescription"]),
                sql_bool(row["controlled"]),
            ]
        )
        + ")"
    )

up_sql = f"""-- Generated from deploy/seed/medicines.csv.
INSERT INTO catalog_molecules (molecule_id, molecule_name)
VALUES
{molecule_values}
ON CONFLICT (molecule_id)
DO UPDATE SET molecule_name = EXCLUDED.molecule_name;

INSERT INTO catalog_products (
    product_id,
    brand_name,
    generic_name,
    molecule_id,
    strength,
    dosage_form,
    pack_size,
    manufacturer,
    requires_prescription,
    controlled
)
VALUES
{",\n".join(product_values)}
ON CONFLICT (product_id) DO NOTHING;
"""

product_id_values = ",\n".join(
    f"    {sql_text(row['product_id'].strip())}" for row in rows
)
molecule_id_values = ",\n".join(
    f"    {sql_text(molecule_id)}" for molecule_id in molecules
)

down_sql = f"""-- Remove the products imported by the catalog seed migration.
DELETE FROM catalog_products
WHERE product_id IN (
{product_id_values}
);

DELETE FROM catalog_molecules
WHERE molecule_id IN (
{molecule_id_values}
)
AND NOT EXISTS (
    SELECT 1
    FROM catalog_products p
    WHERE p.molecule_id = catalog_molecules.molecule_id
);
"""

MIGRATIONS.mkdir(parents=True, exist_ok=True)
(MIGRATIONS / "002_seed_medicines.up.sql").write_text(up_sql, encoding="utf-8")
(MIGRATIONS / "002_seed_medicines.down.sql").write_text(down_sql, encoding="utf-8")

print(f"Generated migrations for {len(rows)} medicines and {len(molecules)} molecules.")